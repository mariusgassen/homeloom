package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3filter"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Which operations need a token is decided by the `security` section of the OpenAPI spec:
// the request validator calls authenticate for those operations only. The validator cannot
// change the request context, so withAuthSlot puts a slot there for authenticate to fill.

type authSlot struct {
	user domain.User
	ok   bool
	err  error
}

type slotKey struct{}

func withAuthSlot(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), slotKey{}, &authSlot{})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func slotFrom(ctx context.Context) *authSlot {
	s, _ := ctx.Value(slotKey{}).(*authSlot)
	return s
}

func authenticate(identity port.IdentityService) openapi3filter.AuthenticationFunc {
	return func(ctx context.Context, in *openapi3filter.AuthenticationInput) error {
		slot := slotFrom(ctx)
		if slot == nil {
			return errors.New("auth slot missing")
		}
		req := in.RequestValidationInput.Request
		var user domain.User
		var err error
		switch in.SecuritySchemeName {
		case "bearerAuth":
			token, found := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
			if !found {
				return slot.fail(in, domain.ErrUnauthenticated)
			}
			user, err = identity.Authenticate(ctx, strings.TrimSpace(token))
		case "cookieAuth":
			c, cerr := req.Cookie(sessionCookie)
			if cerr != nil {
				return slot.fail(in, domain.ErrUnauthenticated)
			}
			user, err = identity.AuthenticateSession(ctx, c.Value)
		default:
			return fmt.Errorf("unknown security scheme %q", in.SecuritySchemeName)
		}
		if err != nil {
			return slot.fail(in, err)
		}
		slot.user, slot.ok, slot.err = user, true, nil
		return nil
	}
}

// fail records why a scheme was rejected. The spec offers bearer or cookie, and the validator
// tries both, so a missing credential must not hide that the other one could not be checked.
func (s *authSlot) fail(in *openapi3filter.AuthenticationInput, err error) error {
	if s.err == nil || errors.Is(s.err, domain.ErrUnauthenticated) {
		s.err = err
	}
	return in.NewError(err)
}

// actor returns the authenticated user. Only call it from operations that require auth.
func actor(ctx context.Context) (domain.User, error) {
	if s := slotFrom(ctx); s != nil && s.ok {
		return s.user, nil
	}
	return domain.User{}, domain.ErrUnauthenticated
}
