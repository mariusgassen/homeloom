// Package i18n holds Homeloom's server-side translations and language negotiation.
// Clients translate their own UI; the server translates what it writes itself:
// error messages now, notifications and generated texts later.
package i18n

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"golang.org/x/text/language"
)

// Supported lists the languages Homeloom ships, the fallback first.
var Supported = []language.Tag{language.English, language.German}

var matcher = language.NewMatcher(Supported)

//go:embed locales/*.json
var files embed.FS

var catalogs = mustLoad()

func mustLoad() map[language.Tag]map[string]string {
	out := map[language.Tag]map[string]string{}
	for _, tag := range Supported {
		raw, err := files.ReadFile(path.Join("locales", tag.String()+".json"))
		if err != nil {
			panic(err)
		}
		var msgs map[string]string
		if err := json.Unmarshal(raw, &msgs); err != nil {
			panic(fmt.Errorf("i18n: %s: %w", tag, err))
		}
		out[tag] = msgs
	}
	return out
}

// Negotiate picks the best supported language for an Accept-Language header value.
func Negotiate(acceptLanguage string) language.Tag {
	prefs, _, _ := language.ParseAcceptLanguage(acceptLanguage)
	_, i, _ := matcher.Match(prefs...)
	return Supported[i]
}

// T returns the message for key in tag, falling back to English and then to the key itself.
// Placeholders are written {name} and filled from args given as name, value pairs.
func T(tag language.Tag, key string, args ...any) string {
	msg, ok := catalogs[tag][key]
	if !ok {
		msg, ok = catalogs[Supported[0]][key]
	}
	if !ok {
		return key
	}
	for i := 0; i+1 < len(args); i += 2 {
		msg = strings.ReplaceAll(msg, "{"+fmt.Sprint(args[i])+"}", fmt.Sprint(args[i+1]))
	}
	return msg
}

type ctxKey struct{}

func WithLanguage(ctx context.Context, tag language.Tag) context.Context {
	return context.WithValue(ctx, ctxKey{}, tag)
}

// FromContext returns the request's language, or the fallback when none was negotiated.
func FromContext(ctx context.Context) language.Tag {
	if tag, ok := ctx.Value(ctxKey{}).(language.Tag); ok {
		return tag
	}
	return Supported[0]
}
