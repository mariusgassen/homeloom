package i18n

import (
	"testing"

	"golang.org/x/text/language"
)

func TestCatalogsHaveTheSameKeys(t *testing.T) {
	ref := catalogs[Supported[0]]
	for _, tag := range Supported[1:] {
		for key := range ref {
			if _, ok := catalogs[tag][key]; !ok {
				t.Errorf("%s is missing %q", tag, key)
			}
		}
		for key := range catalogs[tag] {
			if _, ok := ref[key]; !ok {
				t.Errorf("%s has %q, which %s lacks", tag, key, Supported[0])
			}
		}
	}
}

func TestNegotiate(t *testing.T) {
	cases := map[string]language.Tag{
		"":                        language.English,
		"de-DE,de;q=0.9,en;q=0.8": language.German,
		"de-AT":                   language.German,
		"en-GB":                   language.English,
		"fr-FR,de;q=0.5":          language.German,
		"fr-FR":                   language.English,
	}
	for header, want := range cases {
		if got := Negotiate(header); got != want {
			t.Errorf("Negotiate(%q) = %s, want %s", header, got, want)
		}
	}
}

func TestT(t *testing.T) {
	if got := T(language.German, "error.invalid_household_name", "max", 100); got != "Gib dem Haushalt einen Namen mit 1 bis 100 Zeichen." {
		t.Errorf("got %q", got)
	}
	if got := T(language.German, "no.such.key"); got != "no.such.key" {
		t.Errorf("missing key should echo the key, got %q", got)
	}
}
