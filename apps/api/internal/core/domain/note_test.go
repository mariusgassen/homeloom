package domain_test

import (
	"strings"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func TestNoteTitle(t *testing.T) {
	for text, want := range map[string]string{
		"# Pasta night\n\nWith **basil**":  "Pasta night",
		"\n\n## Packing  list ##\n- socks": "Packing list",
		"![beach](/v1/x)\nSummer **2026**": "Summer 2026",
		"- [ ] call [Oma](https://x.de)":   "call Oma",
		"> *Remember* this":                "Remember this",
		"just a line\nand more":            "just a line",
		"   \n":                            "",
		"#hashtag first":                   "hashtag first",
		strings.Repeat("a", 120):           strings.Repeat("a", 100),
	} {
		if got := domain.NoteTitle(text); got != want {
			t.Errorf("NoteTitle(%q) = %q, want %q", text, got, want)
		}
	}
}
