package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	MaxDocumentName = 200
	// MaxDocumentBytes is the largest file a document can be; files are sent as they are.
	MaxDocumentBytes = 50 << 20
	// MaxDocuments is how many documents one list returns.
	MaxDocuments = 500
)

// ErrDocument means a file of a type documents don't take, an empty file, or one over MaxDocumentBytes.
var ErrDocument = fmt.Errorf("%w: a document must be a PDF, image, text or office file of at most %d MB", ErrInvalid, MaxDocumentBytes>>20)

// Document is a file a member keeps in the vault. It is private to its owner until they share it
// with the household. The file itself lives in the file store.
type Document struct {
	ID          uuid.UUID
	HouseholdID uuid.UUID
	OwnerID     uuid.UUID
	Name        string
	ContentType string
	Size        int64
	Shared      bool
	CreatedAt   time.Time
}
