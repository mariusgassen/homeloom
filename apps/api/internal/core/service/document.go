package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// documentTypes are the files the vault takes. Each is served back with its own type, so the list
// stays to types a browser shows safely or downloads.
var documentTypes = map[string]bool{
	"application/pdf": true,
	"image/jpeg":      true, "image/png": true, "image/webp": true, "image/heic": true,
	"text/plain": true, "text/csv": true,
	"application/msword": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"application/vnd.ms-excel": true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.ms-powerpoint":                                             true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	"application/vnd.oasis.opendocument.text":                                   true,
	"application/vnd.oasis.opendocument.spreadsheet":                            true,
}

// Documents is the vault: files that are private to whoever added them until they share them.
type Documents struct {
	households port.HouseholdRepository
	documents  port.DocumentRepository
	// files is where the files go; without it the vault is unavailable.
	files port.FileStore
}

func NewDocuments(households port.HouseholdRepository, documents port.DocumentRepository, files port.FileStore) *Documents {
	return &Documents{households: households, documents: documents, files: files}
}

func (s *Documents) List(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.Document, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	return s.documents.Documents(ctx, householdID, actor.ID, domain.MaxDocuments)
}

func (s *Documents) Add(ctx context.Context, actor domain.User, householdID uuid.UUID, name, contentType string, r io.Reader) (domain.Document, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.Document{}, err
	}
	name, err := documentName(name)
	if err != nil {
		return domain.Document{}, err
	}
	if s.files == nil {
		return domain.Document{}, fmt.Errorf("%w: documents need the S3 store", domain.ErrUnavailable)
	}
	if !documentTypes[contentType] {
		return domain.Document{}, domain.ErrDocument
	}
	br := bufio.NewReader(r)
	if _, err := br.Peek(1); err != nil {
		// Nothing to keep, or a body that broke before it began.
		if errors.Is(err, io.EOF) {
			return domain.Document{}, domain.ErrDocument
		}
		return domain.Document{}, err
	}
	if contentType == "application/pdf" {
		if head, err := br.Peek(5); err != nil || string(head) != "%PDF-" {
			return domain.Document{}, domain.ErrDocument
		}
	}
	id := uuid.New()
	counted := &countedReader{r: br, left: domain.MaxDocumentBytes}
	if err := s.files.PutVideo(ctx, documentKey(id), contentType, counted); err != nil {
		if errors.Is(err, domain.ErrDocument) {
			return domain.Document{}, domain.ErrDocument
		}
		return domain.Document{}, err
	}
	d, err := s.documents.CreateDocument(ctx, domain.Document{
		ID: id, HouseholdID: householdID, OwnerID: actor.ID, Name: name, ContentType: contentType, Size: counted.n,
	})
	if err != nil {
		// Don't leave a file nothing points to.
		_ = s.files.DeleteVideo(context.WithoutCancel(ctx), documentKey(id))
		return domain.Document{}, err
	}
	return d, nil
}

func (s *Documents) Update(ctx context.Context, actor domain.User, householdID, id uuid.UUID, name string, shared *bool) (domain.Document, error) {
	d, err := s.own(ctx, actor, householdID, id)
	if err != nil {
		return domain.Document{}, err
	}
	if name != "" {
		if name, err = documentName(name); err != nil {
			return domain.Document{}, err
		}
		d.Name = name
	}
	if shared != nil {
		d.Shared = *shared
	}
	return s.documents.UpdateDocument(ctx, householdID, actor.ID, id, d.Name, d.Shared)
}

func (s *Documents) Delete(ctx context.Context, actor domain.User, householdID, id uuid.UUID) error {
	if _, err := s.own(ctx, actor, householdID, id); err != nil {
		return err
	}
	// The row goes first: a file left behind is only wasted space, a row without its file is a broken document.
	if err := s.documents.DeleteDocument(ctx, householdID, actor.ID, id); err != nil {
		return err
	}
	if s.files == nil {
		return nil
	}
	return s.files.DeleteVideo(ctx, documentKey(id))
}

func (s *Documents) File(ctx context.Context, actor domain.User, householdID, id uuid.UUID) (domain.Document, domain.VideoStream, error) {
	d, err := s.visible(ctx, actor, householdID, id)
	if err != nil {
		return domain.Document{}, domain.VideoStream{}, err
	}
	if s.files == nil {
		return domain.Document{}, domain.VideoStream{}, domain.ErrNotFound
	}
	key := documentKey(id)
	v, err := s.files.StatVideo(ctx, key)
	if err != nil {
		return domain.Document{}, domain.VideoStream{}, err
	}
	return d, domain.VideoStream{Video: v, Open: func(ctx context.Context, offset, length int64) (io.ReadCloser, error) {
		return s.files.OpenVideo(ctx, key, offset, length)
	}}, nil
}

// visible is the document if the actor is a member who owns it or to whom it is shared; anyone
// else gets domain.ErrNotFound, so ids can't be probed.
func (s *Documents) visible(ctx context.Context, actor domain.User, householdID, id uuid.UUID) (domain.Document, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.Document{}, err
	}
	return s.documents.GetDocument(ctx, householdID, actor.ID, id)
}

// own is the actor's document; one shared by someone else is domain.ErrForbidden.
func (s *Documents) own(ctx context.Context, actor domain.User, householdID, id uuid.UUID) (domain.Document, error) {
	d, err := s.visible(ctx, actor, householdID, id)
	if err != nil {
		return domain.Document{}, err
	}
	if d.OwnerID != actor.ID {
		return domain.Document{}, domain.ErrForbidden
	}
	return d, nil
}

func documentName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > domain.MaxDocumentName {
		return "", domain.TextError{Max: domain.MaxDocumentName}
	}
	return name, nil
}

// documentKey is where a document's file is kept in the FileStore.
func documentKey(id uuid.UUID) string { return "documents/" + id.String() }

// countedReader reads at most left bytes, failing with domain.ErrDocument on more so a file that
// is too big is stopped while it is sent, and counts what it read.
type countedReader struct {
	r    io.Reader
	left int64
	n    int64
}

func (c *countedReader) Read(p []byte) (int, error) {
	if c.left <= 0 {
		// One byte more tells a file that is exactly the limit from a bigger one.
		var b [1]byte
		n, err := c.r.Read(b[:])
		if n > 0 {
			return 0, domain.ErrDocument
		}
		return 0, err
	}
	if int64(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.r.Read(p)
	c.left -= int64(n)
	c.n += int64(n)
	return n, err
}
