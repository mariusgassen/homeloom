// Package notesdav is the driving WebDAV adapter for notes: it serves each member's notes as
// Markdown files at /webdav, in a Shared and a Private folder, so notes apps that sync over WebDAV
// read and change them. It signs in with a personal notes password (any user name), which acts as
// its owner in one household. "Later" notes are left out.
package notesdav

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/emersion/go-webdav"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

const (
	// Prefix is where the notes are served.
	Prefix = "/webdav"
	// maxBody bounds one file an app sends; a note holds at most domain.MaxNoteText characters.
	maxBody = 4 * domain.MaxNoteText
	// maxTitle is how much of a note's first line names its file.
	maxTitle = 60
	mimeType = "text/markdown; charset=utf-8"
)

type Deps struct {
	Agents  port.AgentAccessService
	Entries port.EntrySyncService
}

// folders are the lists of notes, by folder name.
var folders = []struct {
	name string
	list domain.SyncList
}{
	{"Shared", domain.SyncList{Kind: domain.KindNote, Visibility: domain.VisibilityShared}},
	{"Private", domain.SyncList{Kind: domain.KindNote, Visibility: domain.VisibilityPrivate}},
}

type grantKey struct{}

// NewHandler serves the notes at Prefix.
func NewHandler(d Deps) http.Handler {
	h := &webdav.Handler{FileSystem: &fs{d}}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The password is a personal token; the user name means nothing.
		_, token, _ := r.BasicAuth()
		g, err := d.Agents.AuthenticatePassword(r.Context(), domain.AccessNotes, token)
		switch {
		case errors.Is(err, domain.ErrUnauthenticated):
			w.Header().Set("WWW-Authenticate", `Basic realm="Homeloom notes", charset="UTF-8"`)
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		case err != nil:
			slog.ErrorContext(r.Context(), "webdav authentication unavailable", "err", err)
			http.Error(w, "authentication unavailable", http.StatusServiceUnavailable)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), grantKey{}, g))
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		h.ServeHTTP(w, r)
	})
}

func grant(ctx context.Context) domain.AgentGrant {
	g, _ := ctx.Value(grantKey{}).(domain.AgentGrant)
	return g
}

type fs struct{ Deps }

// where is a path split into its folder ("" for the root) and file name ("" for a folder).
type where struct {
	folder string
	list   domain.SyncList
	file   string
}

func parse(p string) (where, error) {
	rest, ok := strings.CutPrefix(path.Clean("/"+p), Prefix)
	if !ok || rest != "" && rest[0] != '/' {
		return where{}, notFound
	}
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		return where{}, nil
	}
	folder, file, _ := strings.Cut(rest, "/")
	for _, f := range folders {
		if f.name == folder && !strings.Contains(file, "/") {
			return where{folder: f.name, list: f.list, file: file}, nil
		}
	}
	return where{}, notFound
}

var (
	notFound  = webdav.NewHTTPError(http.StatusNotFound, nil)
	forbidden = webdav.NewHTTPError(http.StatusForbidden, errors.New("only .md and .txt files directly in Shared or Private"))
)

func dirInfo(folder string) webdav.FileInfo {
	p := Prefix + "/"
	if folder != "" {
		p += folder + "/"
	}
	return webdav.FileInfo{Path: p, IsDir: true}
}

// file is a note as the app sees it.
type file struct {
	info webdav.FileInfo
	note domain.Entry
}

func fileOf(folder, name string, e domain.Entry) file {
	sum := sha256.Sum256([]byte(e.Text))
	return file{note: e, info: webdav.FileInfo{
		Path: Prefix + "/" + folder + "/" + name, Size: int64(len(e.Text)), ModTime: e.UpdatedAt,
		MIMEType: mimeType, ETag: hex.EncodeToString(sum[:16]),
	}}
}

// files lists a folder. A note an app made keeps the name the app gave it; one made in Homeloom is
// named after its first line, numbered when two would share a name.
func (f *fs) files(ctx context.Context, w where) ([]file, error) {
	g := grant(ctx)
	es, err := f.Entries.SyncEntries(ctx, g.User, g.HouseholdID, w.list)
	if err != nil {
		return nil, httpError(ctx, err)
	}
	slices.SortFunc(es, func(a, b domain.Entry) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), strings.Compare(a.ID.String(), b.ID.String()))
	})
	taken := map[string]bool{}
	for _, e := range es {
		taken[strings.ToLower(e.ICalName)] = e.ICalName != ""
	}
	out := make([]file, 0, len(es))
	for _, e := range es {
		name := e.ICalName
		if name == "" {
			name = freeName(title(e.Text), taken)
		}
		out = append(out, fileOf(w.folder, name, e))
	}
	return out, nil
}

// title makes a file name from a note's first line.
func title(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	line = strings.TrimLeft(line, "# ")
	line = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
			return ' '
		}
		return r
	}, line)
	line = strings.Join(strings.Fields(line), " ")
	if utf8.RuneCountInString(line) > maxTitle {
		line = strings.TrimSpace(string([]rune(line)[:maxTitle]))
	}
	line = strings.TrimLeft(line, ".")
	if line == "" {
		line = "Note"
	}
	return line
}

func freeName(base string, taken map[string]bool) string {
	name := base + ".md"
	for n := 2; taken[strings.ToLower(name)]; n++ {
		name = fmt.Sprintf("%s %d.md", base, n)
	}
	taken[strings.ToLower(name)] = true
	return name
}

func (f *fs) find(ctx context.Context, w where) (file, error) {
	list, err := f.files(ctx, w)
	if err != nil {
		return file{}, err
	}
	for _, x := range list {
		if path.Base(x.info.Path) == w.file {
			return x, nil
		}
	}
	return file{}, notFound
}

func (f *fs) Open(ctx context.Context, name string) (io.ReadCloser, error) {
	w, err := parse(name)
	if err != nil {
		return nil, err
	}
	if w.file == "" {
		return nil, webdav.NewHTTPError(http.StatusMethodNotAllowed, nil)
	}
	x, err := f.find(ctx, w)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(strings.NewReader(x.note.Text)), nil
}

func (f *fs) Stat(ctx context.Context, name string) (*webdav.FileInfo, error) {
	w, err := parse(name)
	if err != nil {
		return nil, err
	}
	if w.file == "" {
		info := dirInfo(w.folder)
		return &info, nil
	}
	x, err := f.find(ctx, w)
	if err != nil {
		return nil, err
	}
	return &x.info, nil
}

func (f *fs) ReadDir(ctx context.Context, name string, recursive bool) ([]webdav.FileInfo, error) {
	w, err := parse(name)
	if err != nil {
		return nil, err
	}
	if w.file != "" {
		return nil, webdav.NewHTTPError(http.StatusMethodNotAllowed, nil)
	}
	var dirs []where
	out := []webdav.FileInfo{dirInfo(w.folder)}
	if w.folder == "" {
		for _, d := range folders {
			out = append(out, dirInfo(d.name))
			if recursive {
				dirs = append(dirs, where{folder: d.name, list: d.list})
			}
		}
	} else {
		dirs = []where{w}
	}
	for _, d := range dirs {
		list, err := f.files(ctx, d)
		if err != nil {
			return nil, err
		}
		for _, x := range list {
			out = append(out, x.info)
		}
	}
	return out, nil
}

func (f *fs) Create(ctx context.Context, name string, body io.ReadCloser, opts *webdav.CreateOptions) (*webdav.FileInfo, bool, error) {
	w, err := f.writable(name)
	if err != nil {
		return nil, false, err
	}
	b, err := io.ReadAll(body)
	if err != nil {
		return nil, false, webdav.NewHTTPError(http.StatusRequestEntityTooLarge, err)
	}
	existing, err := f.find(ctx, w)
	exists := err == nil
	if err != nil && !errors.Is(err, notFound) {
		return nil, false, err
	}
	if err := conditions(existing.info.ETag, exists, opts.IfMatch, opts.IfNoneMatch); err != nil {
		return nil, false, err
	}
	return f.put(ctx, w, existing, exists, b)
}

// put stores text as the note at w: the existing one, or a new one named as the app named it.
func (f *fs) put(ctx context.Context, w where, existing file, exists bool, b []byte) (*webdav.FileInfo, bool, error) {
	text := strings.ToValidUTF8(string(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))), "")
	if strings.TrimSpace(text) == "" {
		// Apps make empty files for new notes; a note needs text, so it starts with its name.
		text = strings.TrimSuffix(w.file, path.Ext(w.file))
	}
	syncName := w.file
	if exists {
		syncName = existing.note.SyncName()
	}
	g := grant(ctx)
	e, created, err := f.Entries.SyncPut(ctx, g.User, g.HouseholdID, w.list, syncName, domain.SyncEntry{EntryFields: domain.EntryFields{Text: text}})
	if err != nil {
		return nil, false, httpError(ctx, err)
	}
	if e.ICalName == "" {
		// A note named after its first line keeps its name once an app writes it, so the app
		// finds it where it put it.
		if e, err = f.Entries.SyncMove(ctx, g.User, g.HouseholdID, w.list, syncName, w.list, w.file); err != nil {
			return nil, false, httpError(ctx, err)
		}
	}
	info := fileOf(w.folder, e.ICalName, e).info
	return &info, created, nil
}

func (f *fs) RemoveAll(ctx context.Context, name string, opts *webdav.RemoveAllOptions) error {
	w, err := parse(name)
	if err != nil {
		return err
	}
	if w.file == "" {
		return webdav.NewHTTPError(http.StatusForbidden, errors.New("the folders are fixed"))
	}
	x, err := f.find(ctx, w)
	if err != nil {
		return err
	}
	if err := conditions(x.info.ETag, true, opts.IfMatch, opts.IfNoneMatch); err != nil {
		return err
	}
	g := grant(ctx)
	return httpError(ctx, f.Entries.SyncDelete(ctx, g.User, g.HouseholdID, w.list, x.note.SyncName()))
}

func (f *fs) Mkdir(_ context.Context, name string) error {
	if w, err := parse(name); err == nil && w.file == "" {
		return webdav.NewHTTPError(http.StatusMethodNotAllowed, errors.New("already there"))
	}
	return webdav.NewHTTPError(http.StatusForbidden, errors.New("the folders are fixed"))
}

func (f *fs) Copy(ctx context.Context, name, dest string, opts *webdav.CopyOptions) (bool, error) {
	from, err := parse(name)
	if err != nil {
		return false, err
	}
	to, err := f.writable(dest)
	if err != nil || from.file == "" {
		return false, cmp.Or(err, forbidden)
	}
	x, err := f.find(ctx, from)
	if err != nil {
		return false, err
	}
	existing, err := f.find(ctx, to)
	exists := err == nil
	if err != nil && !errors.Is(err, notFound) {
		return false, err
	}
	if exists && opts.NoOverwrite {
		return false, webdav.NewHTTPError(http.StatusPreconditionFailed, nil)
	}
	_, created, err := f.put(ctx, to, existing, exists, []byte(x.note.Text))
	return created, err
}

func (f *fs) Move(ctx context.Context, name, dest string, opts *webdav.MoveOptions) (bool, error) {
	from, err := parse(name)
	if err != nil {
		return false, err
	}
	to, err := f.writable(dest)
	if err != nil || from.file == "" {
		return false, cmp.Or(err, forbidden)
	}
	x, err := f.find(ctx, from)
	if err != nil {
		return false, err
	}
	existing, err := f.find(ctx, to)
	switch {
	case err == nil && existing.note.ID == x.note.ID:
		return false, nil
	case err == nil && opts.NoOverwrite:
		return false, webdav.NewHTTPError(http.StatusPreconditionFailed, nil)
	case err == nil:
		// Replace what is there, as WebDAV's MOVE does.
		g := grant(ctx)
		if err := f.Entries.SyncDelete(ctx, g.User, g.HouseholdID, to.list, existing.note.SyncName()); err != nil {
			return false, httpError(ctx, err)
		}
	case !errors.Is(err, notFound):
		return false, err
	}
	g := grant(ctx)
	if _, err := f.Entries.SyncMove(ctx, g.User, g.HouseholdID, from.list, x.note.SyncName(), to.list, to.file); err != nil {
		return false, httpError(ctx, err)
	}
	return err != nil, nil
}

// writable is a path a note can be put at: a .md or .txt file in one of the folders, not hidden.
func (f *fs) writable(name string) (where, error) {
	w, err := parse(name)
	if err != nil {
		return where{}, forbidden
	}
	ext := strings.ToLower(path.Ext(w.file))
	if w.folder == "" || w.file == "" || strings.HasPrefix(w.file, ".") || ext != ".md" && ext != ".txt" ||
		utf8.RuneCountInString(w.file) > domain.MaxICalName {
		return where{}, forbidden
	}
	return w, nil
}

// conditions checks If-Match and If-None-Match against the file's ETag.
func conditions(etag string, exists bool, ifMatch, ifNoneMatch webdav.ConditionalMatch) error {
	failed := webdav.NewHTTPError(http.StatusPreconditionFailed, nil)
	if ifNoneMatch.IsSet() && exists {
		if ok, err := ifNoneMatch.MatchETag(etag); err != nil || ok {
			return failed
		}
	}
	if ifMatch.IsSet() {
		if !exists {
			return failed
		}
		if ok, err := ifMatch.MatchETag(etag); err != nil || !ok {
			return failed
		}
	}
	return nil
}

// httpError turns a core error into the status an app understands; others are logged, not shown.
func httpError(ctx context.Context, err error) error {
	var text domain.TextError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return notFound
	case errors.As(err, &text):
		return webdav.NewHTTPError(http.StatusRequestEntityTooLarge, err)
	case errors.Is(err, domain.ErrForbidden):
		return webdav.NewHTTPError(http.StatusForbidden, err)
	case errors.Is(err, domain.ErrConflict):
		return webdav.NewHTTPError(http.StatusConflict, err)
	case errors.Is(err, domain.ErrInvalid):
		return webdav.NewHTTPError(http.StatusBadRequest, err)
	}
	slog.ErrorContext(ctx, "webdav request failed", "err", err)
	return webdav.NewHTTPError(http.StatusInternalServerError, errors.New("internal error"))
}
