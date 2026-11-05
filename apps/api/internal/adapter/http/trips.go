package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (h *handlers) ListTrips(ctx context.Context, req oapi.ListTripsRequestObject) (oapi.ListTripsResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	ts, err := h.trips.List(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ListTrips404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	out := make(oapi.ListTrips200JSONResponse, len(ts))
	for i, t := range ts {
		out[i] = toTrip(t)
	}
	return out, nil
}

func (h *handlers) CreateTrip(ctx context.Context, req oapi.CreateTripRequestObject) (oapi.CreateTripResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	t, err := h.trips.Create(ctx, u, req.HouseholdId, toTripFields(*req.Body))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.CreateTrip404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrConflict):
		return oapi.CreateTrip409JSONResponse{TripFullJSONResponse: oapi.TripFullJSONResponse(tripFull(ctx, err))}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.CreateTrip400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidTrip(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.CreateTrip201JSONResponse(toTrip(t)), nil
}

func (h *handlers) UpdateTrip(ctx context.Context, req oapi.UpdateTripRequestObject) (oapi.UpdateTripResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	t, err := h.trips.Update(ctx, u, req.HouseholdId, req.TripId, toTripFields(*req.Body))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UpdateTrip404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.UpdateTrip400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidTrip(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.UpdateTrip200JSONResponse(toTrip(t)), nil
}

func (h *handlers) DeleteTrip(ctx context.Context, req oapi.DeleteTripRequestObject) (oapi.DeleteTripResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.trips.Delete(ctx, u, req.HouseholdId, req.TripId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteTrip404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.DeleteTrip503JSONResponse{UnavailableJSONResponse: photosUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteTrip204Response{}, nil
}

func (h *handlers) AddTripOption(ctx context.Context, req oapi.AddTripOptionRequestObject) (oapi.AddTripOptionResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	t, err := h.trips.AddOption(ctx, u, req.HouseholdId, req.TripId, toTripOptionFields(*req.Body))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.AddTripOption404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrConflict):
		return oapi.AddTripOption409JSONResponse{TripFullJSONResponse: oapi.TripFullJSONResponse(tripFull(ctx, err))}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.AddTripOption400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidTrip(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.AddTripOption201JSONResponse(toTrip(t)), nil
}

func (h *handlers) UpdateTripOption(ctx context.Context, req oapi.UpdateTripOptionRequestObject) (oapi.UpdateTripOptionResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	t, err := h.trips.UpdateOption(ctx, u, req.HouseholdId, req.TripId, req.OptionId, toTripOptionFields(*req.Body))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UpdateTripOption404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.UpdateTripOption400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidTrip(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.UpdateTripOption200JSONResponse(toTrip(t)), nil
}

func (h *handlers) DeleteTripOption(ctx context.Context, req oapi.DeleteTripOptionRequestObject) (oapi.DeleteTripOptionResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	t, err := h.trips.DeleteOption(ctx, u, req.HouseholdId, req.TripId, req.OptionId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteTripOption404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.DeleteTripOption503JSONResponse{UnavailableJSONResponse: photosUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteTripOption200JSONResponse(toTrip(t)), nil
}

func (h *handlers) VoteTripOption(ctx context.Context, req oapi.VoteTripOptionRequestObject) (oapi.VoteTripOptionResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	t, err := h.trips.Vote(ctx, u, req.HouseholdId, req.TripId, req.OptionId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.VoteTripOption404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.VoteTripOption200JSONResponse(toTrip(t)), nil
}

func (h *handlers) UnvoteTrip(ctx context.Context, req oapi.UnvoteTripRequestObject) (oapi.UnvoteTripResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	t, err := h.trips.Unvote(ctx, u, req.HouseholdId, req.TripId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UnvoteTrip404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.UnvoteTrip200JSONResponse(toTrip(t)), nil
}

func (h *handlers) AddTripPhoto(ctx context.Context, req oapi.AddTripPhotoRequestObject) (oapi.AddTripPhotoResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	// One byte more than allowed is enough to tell the photo is too big.
	data, err := io.ReadAll(io.LimitReader(req.Body, domain.MaxPhotoBytes+1))
	if err != nil {
		return oapi.AddTripPhoto400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_photo", err.Error(), "max", domain.MaxPhotoBytes>>20))}, nil
	}
	t, err := h.trips.AddPhoto(ctx, u, req.HouseholdId, req.TripId, req.OptionId, domain.Photo{ContentType: "image/jpeg", Data: data})
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.AddTripPhoto404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrConflict):
		return oapi.AddTripPhoto409JSONResponse{TripFullJSONResponse: oapi.TripFullJSONResponse(tripFull(ctx, err))}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.AddTripPhoto400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_photo", err.Error(), "max", domain.MaxPhotoBytes>>20))}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.AddTripPhoto503JSONResponse{UnavailableJSONResponse: photosUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.AddTripPhoto201JSONResponse(toTrip(t)), nil
}

func (h *handlers) GetTripPhoto(ctx context.Context, req oapi.GetTripPhotoRequestObject) (oapi.GetTripPhotoResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	p, err := h.trips.Photo(ctx, u, req.HouseholdId, req.TripId, req.PhotoId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.GetTripPhoto404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.GetTripPhoto503JSONResponse{UnavailableJSONResponse: photosUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	// A photo never changes, and the URL names it, so the browser may keep it; it is private to the viewer.
	cache := "private, max-age=31536000, immutable"
	return oapi.GetTripPhoto200ImagejpegResponse{
		Body: bytes.NewReader(p.Data), ContentLength: int64(len(p.Data)), Headers: oapi.GetTripPhoto200ResponseHeaders{CacheControl: &cache},
	}, nil
}

func (h *handlers) DeleteTripPhoto(ctx context.Context, req oapi.DeleteTripPhotoRequestObject) (oapi.DeleteTripPhotoResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	t, err := h.trips.DeletePhoto(ctx, u, req.HouseholdId, req.TripId, req.PhotoId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteTripPhoto404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.DeleteTripPhoto503JSONResponse{UnavailableJSONResponse: photosUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteTripPhoto200JSONResponse(toTrip(t)), nil
}

func (h *handlers) AddPackingItem(ctx context.Context, req oapi.AddPackingItemRequestObject) (oapi.AddPackingItemResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	t, err := h.trips.AddPacking(ctx, u, req.HouseholdId, req.TripId, req.Body.Text)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.AddPackingItem404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrConflict):
		return oapi.AddPackingItem409JSONResponse{TripFullJSONResponse: oapi.TripFullJSONResponse(tripFull(ctx, err))}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.AddPackingItem400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidTrip(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.AddPackingItem201JSONResponse(toTrip(t)), nil
}

func (h *handlers) UpdatePackingItem(ctx context.Context, req oapi.UpdatePackingItemRequestObject) (oapi.UpdatePackingItemResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	t, err := h.trips.UpdatePacking(ctx, u, req.HouseholdId, req.TripId, req.ItemId, domain.PackingFields{Text: req.Body.Text, Done: req.Body.Done != nil && *req.Body.Done})
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UpdatePackingItem404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.UpdatePackingItem400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidTrip(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.UpdatePackingItem200JSONResponse(toTrip(t)), nil
}

func (h *handlers) DeletePackingItem(ctx context.Context, req oapi.DeletePackingItemRequestObject) (oapi.DeletePackingItemResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	t, err := h.trips.DeletePacking(ctx, u, req.HouseholdId, req.TripId, req.ItemId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeletePackingItem404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.DeletePackingItem200JSONResponse(toTrip(t)), nil
}

func invalidTrip(ctx context.Context, err error) oapi.Error {
	var te domain.TextError
	switch {
	case errors.As(err, &te):
		return errorBody(ctx, "invalid_text", err.Error(), "max", te.Max)
	case errors.Is(err, domain.ErrTripDates):
		return errorBody(ctx, "invalid_trip_dates", err.Error())
	case errors.Is(err, domain.ErrTripOption):
		return errorBody(ctx, "invalid_trip_option", err.Error())
	case errors.Is(err, domain.ErrTripURL):
		return errorBody(ctx, "invalid_trip_url", err.Error())
	}
	return errorBody(ctx, "invalid_trip", err.Error())
}

func tripFull(ctx context.Context, err error) oapi.Error {
	return errorBody(ctx, "trip_full", err.Error())
}

func photosUnavailable(ctx context.Context, err error) oapi.UnavailableJSONResponse {
	slog.WarnContext(ctx, "trip photos unavailable", "err", err)
	return oapi.UnavailableJSONResponse(errorBody(ctx, "photos_unavailable", ""))
}

func toTripFields(b oapi.TripFields) domain.TripFields {
	f := domain.TripFields{Name: b.Name, ChosenOptionID: b.ChosenOptionId}
	if b.Destination != nil {
		f.Destination = *b.Destination
	}
	if b.Notes != nil {
		f.Notes = *b.Notes
	}
	if b.StartOn != nil {
		f.StartOn = &b.StartOn.Time
	}
	if b.EndOn != nil {
		f.EndOn = &b.EndOn.Time
	}
	return f
}

func toTripOptionFields(b oapi.TripOptionFields) domain.TripOptionFields {
	f := domain.TripOptionFields{Title: b.Title}
	if b.Url != nil {
		f.URL = *b.Url
	}
	if b.Notes != nil {
		f.Notes = *b.Notes
	}
	return f
}

func toTrip(t domain.Trip) oapi.Trip {
	out := oapi.Trip{
		Id: t.ID, Name: t.Name, Destination: optional(t.Destination), Notes: optional(t.Notes),
		ChosenOptionId: t.ChosenOptionID, CreatedAt: t.CreatedAt,
		Options: make([]oapi.TripOption, len(t.Options)), Packing: make([]oapi.PackingItem, len(t.Packing)),
	}
	if t.StartOn != nil {
		out.StartOn = &openapi_types.Date{Time: *t.StartOn}
	}
	if t.EndOn != nil {
		out.EndOn = &openapi_types.Date{Time: *t.EndOn}
	}
	for i, o := range t.Options {
		oo := oapi.TripOption{
			Id: o.ID, Title: o.Title, Url: optional(o.URL), Notes: optional(o.Notes), AddedBy: o.AddedBy,
			Photos: make([]oapi.TripPhoto, len(o.Photos)), Votes: []openapi_types.UUID{},
		}
		for j, p := range o.Photos {
			oo.Photos[j] = oapi.TripPhoto{Id: p.ID, Url: fmt.Sprintf("/v1/households/%s/trips/%s/photos/%s", t.HouseholdID, t.ID, p.ID)}
		}
		for _, v := range t.Votes {
			if v.OptionID == o.ID {
				oo.Votes = append(oo.Votes, v.UserID)
			}
		}
		out.Options[i] = oo
	}
	for i, p := range t.Packing {
		out.Packing[i] = oapi.PackingItem{Id: p.ID, Text: p.Text, Done: p.Done}
	}
	return out
}
