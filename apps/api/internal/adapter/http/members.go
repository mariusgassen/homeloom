package http

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (h *handlers) SetMemberBirthday(ctx context.Context, req oapi.SetMemberBirthdayRequestObject) (oapi.SetMemberBirthdayResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.members.SetBirthday(ctx, u, req.HouseholdId, req.UserId, domain.Birthday{Month: time.Month(req.Body.Month), Day: req.Body.Day, Year: derefInt(req.Body.Year)})
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.SetMemberBirthday404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return oapi.SetMemberBirthday403JSONResponse(errorBody(ctx, "not_owner", err.Error())), nil
	case errors.Is(err, domain.ErrBirthday):
		return oapi.SetMemberBirthday400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_birthday", err.Error()))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.SetMemberBirthday204Response{}, nil
}

func (h *handlers) DeleteMemberBirthday(ctx context.Context, req oapi.DeleteMemberBirthdayRequestObject) (oapi.DeleteMemberBirthdayResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.members.ClearBirthday(ctx, u, req.HouseholdId, req.UserId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteMemberBirthday404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return oapi.DeleteMemberBirthday403JSONResponse(errorBody(ctx, "not_owner", err.Error())), nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteMemberBirthday204Response{}, nil
}

func (h *handlers) SetMemberAvatar(ctx context.Context, req oapi.SetMemberAvatarRequestObject) (oapi.SetMemberAvatarResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	bad := oapi.SetMemberAvatar400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_photo", domain.ErrAvatar.Error(), "max", domain.MaxPhotoBytes>>20))}
	data, err := io.ReadAll(io.LimitReader(req.Body, domain.MaxPhotoBytes+1))
	if err != nil {
		return bad, nil
	}
	m, err := h.members.SetAvatar(ctx, u, req.HouseholdId, req.UserId, domain.Photo{ContentType: "image/jpeg", Data: data})
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.SetMemberAvatar404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return oapi.SetMemberAvatar403JSONResponse(errorBody(ctx, "not_owner", err.Error())), nil
	case errors.Is(err, domain.ErrAvatar):
		return bad, nil
	case err != nil:
		return nil, err
	}
	return oapi.SetMemberAvatar200JSONResponse(toUser(m)), nil
}

func (h *handlers) DeleteMemberAvatar(ctx context.Context, req oapi.DeleteMemberAvatarRequestObject) (oapi.DeleteMemberAvatarResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	m, err := h.members.DeleteAvatar(ctx, u, req.HouseholdId, req.UserId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteMemberAvatar404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return oapi.DeleteMemberAvatar403JSONResponse(errorBody(ctx, "not_owner", err.Error())), nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteMemberAvatar200JSONResponse(toUser(m)), nil
}
