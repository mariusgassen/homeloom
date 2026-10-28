package http

import (
	"context"
	"errors"
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
		token, found := strings.CutPrefix(in.RequestValidationInput.Request.Header.Get("Authorization"), "Bearer ")
		if !found {
			slot.err = domain.ErrUnauthenticated
			return in.NewError(slot.err)
		}
		user, err := identity.Authenticate(ctx, strings.TrimSpace(token))
		if err != nil {
			slot.err = err
			return in.NewError(err)
		}
		slot.user, slot.ok = user, true
		return nil
	}
}

// actor returns the authenticated user. Only call it from operations that require auth.
func actor(ctx context.Context) (domain.User, error) {
	if s := slotFrom(ctx); s != nil && s.ok {
		return s.user, nil
	}
	return domain.User{}, domain.ErrUnauthenticated
}
