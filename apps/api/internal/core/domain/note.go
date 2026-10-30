package domain

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxNotePhotos is how many photos one note can hold.
const MaxNotePhotos = 50

// maxNoteTitle is how much of a note's first line its title keeps.
const maxNoteTitle = 100

var (
	// ErrNotePhoto means a photo that isn't a JPEG of at most MaxPhotoBytes, or one on something
	// other than a note.
	ErrNotePhoto = fmt.Errorf("%w: a photo must be a JPEG of at most %d MB, on a note", ErrInvalid, MaxPhotoBytes>>20)
	// ErrNotePhotos means the note already holds MaxNotePhotos photos.
	ErrNotePhotos = fmt.Errorf("%w: a note holds at most %d photos", ErrInvalid, MaxNotePhotos)
)

var (
	// lineMarks are what starts a Markdown heading, quote, list item or task.
	lineMarks = regexp.MustCompile(`^(?:#{1,6}\s+|>\s*|[-*+]\s+(?:\[[ xX]\]\s+)?|\d+[.)]\s+)+`)
	images    = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	links     = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	emphasis  = strings.NewReplacer("*", "", "__", "", "`", "", "~~", "")
)

// NoteTitle is what a note is called: its first line with words, without the Markdown around them,
// so "# Pasta night" is "Pasta night". Photos don't count.
func NoteTitle(text string) string {
	for line := range strings.Lines(text) {
		line = lineMarks.ReplaceAllString(strings.TrimSpace(line), "")
		line = images.ReplaceAllString(line, "")
		line = links.ReplaceAllString(line, "$1")
		line = emphasis.Replace(line)
		line = strings.Join(strings.Fields(strings.Trim(line, "#_ ")), " ")
		if line == "" {
			continue
		}
		if utf8.RuneCountInString(line) > maxNoteTitle {
			line = strings.TrimSpace(string([]rune(line)[:maxNoteTitle]))
		}
		return line
	}
	return ""
}
