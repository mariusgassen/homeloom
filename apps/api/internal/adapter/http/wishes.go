package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (h *handlers) ListWishes(ctx context.Context, req oapi.ListWishesRequestObject) (oapi.ListWishesResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	ws, err := h.wishes.List(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ListWishes404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	out := make(oapi.ListWishes200JSONResponse, len(ws))
	for i, w := range ws {
		out[i] = toWish(w)
	}
	return out, nil
}

func (h *handlers) CreateWish(ctx context.Context, req oapi.CreateWishRequestObject) (oapi.CreateWishResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	w, err := h.wishes.Create(ctx, u, req.HouseholdId, toWishFields(*req.Body))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.CreateWish404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.CreateWish400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidWish(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.CreateWish201JSONResponse(toWish(w)), nil
}

func (h *handlers) UpdateWish(ctx context.Context, req oapi.UpdateWishRequestObject) (oapi.UpdateWishResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	w, err := h.wishes.Update(ctx, u, req.HouseholdId, req.WishId, toWishFields(*req.Body))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UpdateWish404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.UpdateWish400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidWish(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.UpdateWish200JSONResponse(toWish(w)), nil
}

func (h *handlers) DeleteWish(ctx context.Context, req oapi.DeleteWishRequestObject) (oapi.DeleteWishResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.wishes.Delete(ctx, u, req.HouseholdId, req.WishId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteWish404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.DeleteWish503JSONResponse{UnavailableJSONResponse: wishPhotosUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteWish204Response{}, nil
}

func (h *handlers) VoteWish(ctx context.Context, req oapi.VoteWishRequestObject) (oapi.VoteWishResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	w, err := h.wishes.Vote(ctx, u, req.HouseholdId, req.WishId, req.Body.Hearts)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.VoteWish404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.VoteWish400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidWish(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.VoteWish200JSONResponse(toWish(w)), nil
}

func (h *handlers) UnvoteWish(ctx context.Context, req oapi.UnvoteWishRequestObject) (oapi.UnvoteWishResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	w, err := h.wishes.Vote(ctx, u, req.HouseholdId, req.WishId, 0)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UnvoteWish404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.UnvoteWish200JSONResponse(toWish(w)), nil
}

func (h *handlers) GotWish(ctx context.Context, req oapi.GotWishRequestObject) (oapi.GotWishResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	w, err := h.wishes.Got(ctx, u, req.HouseholdId, req.WishId, true)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.GotWish404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.GotWish200JSONResponse(toWish(w)), nil
}

func (h *handlers) UngotWish(ctx context.Context, req oapi.UngotWishRequestObject) (oapi.UngotWishResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	w, err := h.wishes.Got(ctx, u, req.HouseholdId, req.WishId, false)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UngotWish404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.UngotWish200JSONResponse(toWish(w)), nil
}

func (h *handlers) GetWishPhoto(ctx context.Context, req oapi.GetWishPhotoRequestObject) (oapi.GetWishPhotoResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	p, err := h.wishes.Photo(ctx, u, req.HouseholdId, req.WishId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.GetWishPhoto404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.GetWishPhoto503JSONResponse{UnavailableJSONResponse: wishPhotosUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	// photoUrl names the photo's version, so the browser may keep it; it is private to the viewer.
	cache := "private, max-age=31536000, immutable"
	return oapi.GetWishPhoto200ImagejpegResponse{
		Body: bytes.NewReader(p.Data), ContentLength: int64(len(p.Data)), Headers: oapi.GetWishPhoto200ResponseHeaders{CacheControl: &cache},
	}, nil
}

func (h *handlers) SetWishPhoto(ctx context.Context, req oapi.SetWishPhotoRequestObject) (oapi.SetWishPhotoResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	// One byte more than allowed is enough to tell the photo is too big.
	data, err := io.ReadAll(io.LimitReader(req.Body, domain.MaxPhotoBytes+1))
	if err != nil {
		return oapi.SetWishPhoto400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidWishPhoto(ctx, err))}, nil
	}
	w, err := h.wishes.SetPhoto(ctx, u, req.HouseholdId, req.WishId, domain.Photo{ContentType: "image/jpeg", Data: data})
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.SetWishPhoto404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.SetWishPhoto400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidWishPhoto(ctx, err))}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.SetWishPhoto503JSONResponse{UnavailableJSONResponse: wishPhotosUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.SetWishPhoto200JSONResponse(toWish(w)), nil
}

func (h *handlers) DeleteWishPhoto(ctx context.Context, req oapi.DeleteWishPhotoRequestObject) (oapi.DeleteWishPhotoResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	w, err := h.wishes.DeletePhoto(ctx, u, req.HouseholdId, req.WishId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteWishPhoto404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.DeleteWishPhoto503JSONResponse{UnavailableJSONResponse: wishPhotosUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteWishPhoto200JSONResponse(toWish(w)), nil
}

func wishPhotosUnavailable(ctx context.Context, err error) oapi.UnavailableJSONResponse {
	slog.WarnContext(ctx, "wish photos unavailable", "err", err)
	return oapi.UnavailableJSONResponse(errorBody(ctx, "wish_photos_unavailable", ""))
}

func invalidWishPhoto(ctx context.Context, err error) oapi.Error {
	return errorBody(ctx, "invalid_photo", err.Error(), "max", domain.MaxPhotoBytes>>20)
}

// invalidWish names what is wrong with a wish, so the app can say it in the user's language.
func invalidWish(ctx context.Context, err error) oapi.Error {
	var te domain.TextError
	switch {
	case errors.As(err, &te):
		return errorBody(ctx, "invalid_text", err.Error(), "max", te.Max)
	case errors.Is(err, domain.ErrWishNote):
		return errorBody(ctx, "invalid_wish_note", err.Error(), "max", domain.MaxWishNote)
	case errors.Is(err, domain.ErrWishLink):
		return errorBody(ctx, "invalid_wish_link", err.Error())
	case errors.Is(err, domain.ErrWishPrice):
		return errorBody(ctx, "invalid_wish_price", err.Error())
	case errors.Is(err, domain.ErrWishHearts):
		return errorBody(ctx, "invalid_wish_hearts", err.Error())
	}
	return errorBody(ctx, "invalid_wish", err.Error())
}

func toWishFields(b oapi.WishFields) domain.WishFields {
	f := domain.WishFields{Title: b.Title, PriceCents: b.PriceCents}
	if b.Note != nil {
		f.Note = *b.Note
	}
	if b.Url != nil {
		f.URL = *b.Url
	}
	return f
}

func toWish(w domain.Wish) oapi.Wish {
	out := oapi.Wish{
		Id: w.ID, Title: w.Title, Note: w.Note, Url: w.URL, PriceCents: w.PriceCents, Hearts: w.Hearts(),
		Votes: make([]oapi.WishVote, len(w.Votes)), GotAt: w.GotAt, CreatedBy: w.CreatedBy, CreatedAt: w.CreatedAt,
	}
	for i, v := range w.Votes {
		out.Votes[i] = oapi.WishVote{UserId: v.UserID, Hearts: v.Hearts}
	}
	if w.PhotoAt != nil {
		url := fmt.Sprintf("/v1/households/%s/wishes/%s/photo?v=%d", w.HouseholdID, w.ID, w.PhotoAt.UnixMilli())
		out.PhotoUrl = &url
	}
	return out
}
