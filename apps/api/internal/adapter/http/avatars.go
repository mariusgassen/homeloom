package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (h *handlers) SetAvatar(ctx context.Context, req oapi.SetAvatarRequestObject) (oapi.SetAvatarResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	// One byte more than allowed is enough to tell the picture is too big.
	data, err := io.ReadAll(io.LimitReader(req.Body, domain.MaxPhotoBytes+1))
	if err != nil {
		return oapi.SetAvatar400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_photo", err.Error(), "max", domain.MaxPhotoBytes>>20))}, nil
	}
	u, err = h.avatars.Set(ctx, u, domain.Photo{ContentType: "image/jpeg", Data: data})
	if errors.Is(err, domain.ErrAvatar) {
		return oapi.SetAvatar400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_photo", err.Error(), "max", domain.MaxPhotoBytes>>20))}, nil
	}
	if err != nil {
		return nil, err
	}
	return oapi.SetAvatar200JSONResponse(toUser(u)), nil
}

func (h *handlers) DeleteAvatar(ctx context.Context, _ oapi.DeleteAvatarRequestObject) (oapi.DeleteAvatarResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	u, err = h.avatars.Delete(ctx, u)
	if err != nil {
		return nil, err
	}
	return oapi.DeleteAvatar200JSONResponse(toUser(u)), nil
}

func (h *handlers) GetAvatar(ctx context.Context, req oapi.GetAvatarRequestObject) (oapi.GetAvatarResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	p, err := h.avatars.Get(ctx, u, req.UserId)
	if errors.Is(err, domain.ErrNotFound) {
		return oapi.GetAvatar404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	}
	if err != nil {
		return nil, err
	}
	// avatarUrl names the picture's version, so the browser may keep it; it is private to the viewer.
	cache := "private, max-age=31536000, immutable"
	return oapi.GetAvatar200ImagejpegResponse{
		Body: bytes.NewReader(p.Data), ContentLength: int64(len(p.Data)), Headers: oapi.GetAvatar200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// avatarURL is the user's own picture when they set one, else authentik's.
func avatarURL(u domain.User) string {
	if u.AvatarAt != nil {
		return fmt.Sprintf("/v1/users/%s/avatar?v=%d", u.ID, u.AvatarAt.UnixMilli())
	}
	return u.AvatarURL
}
