package http

import (
	"bytes"
	"context"
	"errors"
	"log/slog"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/i18n"
)

func (h *handlers) GetWatchShelf(ctx context.Context, req oapi.GetWatchShelfRequestObject) (oapi.GetWatchShelfResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	shelf, err := h.watch.Shelf(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.GetWatchShelf404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.GetWatchShelf503JSONResponse{UnavailableJSONResponse: watchUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	out := oapi.GetWatchShelf200JSONResponse{Connected: shelf.Connected, Libraries: make([]oapi.WatchLibrary, len(shelf.Libraries))}
	for i, l := range shelf.Libraries {
		out.Libraries[i] = oapi.WatchLibrary{Id: l.ID, Name: l.Name, Kind: oapi.MediaKind(l.Kind), Server: l.Server, Owned: l.Owned}
	}
	return out, nil
}

func (h *handlers) ConnectWatch(ctx context.Context, req oapi.ConnectWatchRequestObject) (oapi.ConnectWatchResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	url, err := h.watch.Connect(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ConnectWatch404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.ConnectWatch503JSONResponse{UnavailableJSONResponse: watchUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.ConnectWatch200JSONResponse{Url: url}, nil
}

func (h *handlers) CompleteWatchConnection(ctx context.Context, req oapi.CompleteWatchConnectionRequestObject) (oapi.CompleteWatchConnectionResponseObject, error) {
	back := func(code string) oapi.CompleteWatchConnection302Response {
		loc := "/watch"
		if code != "" {
			loc += "?watch_error=" + code
		}
		return oapi.CompleteWatchConnection302Response{Headers: oapi.CompleteWatchConnection302ResponseHeaders{Location: loc}}
	}
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.watch.CompleteConnect(ctx, u, deref(req.Params.State))
	switch {
	case errors.Is(err, domain.ErrUnauthenticated):
		slog.InfoContext(ctx, "plex connection rejected", "err", err)
		return back("connect_failed"), nil
	case err != nil:
		slog.WarnContext(ctx, "plex connection failed", "err", err)
		return back("watch_unavailable"), nil
	}
	return back(""), nil
}

func (h *handlers) DisconnectWatch(ctx context.Context, req oapi.DisconnectWatchRequestObject) (oapi.DisconnectWatchResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.watch.Disconnect(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DisconnectWatch404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.DisconnectWatch503JSONResponse{UnavailableJSONResponse: watchUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.DisconnectWatch204Response{}, nil
}

func (h *handlers) ListWatchTitles(ctx context.Context, req oapi.ListWatchTitlesRequestObject) (oapi.ListWatchTitlesResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	titles, err := h.watch.Titles(ctx, u, req.HouseholdId, req.LibraryId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ListWatchTitles404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.ListWatchTitles503JSONResponse{UnavailableJSONResponse: watchUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	out := make(oapi.ListWatchTitles200JSONResponse, len(titles))
	for i, t := range titles {
		out[i] = toWatchTitle(t)
	}
	return out, nil
}

func (h *handlers) GetWatchPoster(ctx context.Context, req oapi.GetWatchPosterRequestObject) (oapi.GetWatchPosterResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	p, err := h.watch.Poster(ctx, u, req.HouseholdId, req.TitleId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.GetWatchPoster404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.GetWatchPoster503JSONResponse{UnavailableJSONResponse: watchUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.GetWatchPoster200ImageResponse{
		Body: bytes.NewReader(p.Body), ContentType: p.ContentType, ContentLength: int64(len(p.Body)),
		// Private: it came with the caller's own access to Plex.
		Headers: oapi.GetWatchPoster200ResponseHeaders{CacheControl: "private, max-age=86400"},
	}, nil
}

func toWatchTitle(t domain.Title) oapi.WatchTitle {
	out := oapi.WatchTitle{
		Id: t.ID, Kind: oapi.MediaKind(t.Kind), Name: t.Name, Genres: t.Genres, Watched: t.Watched,
		Summary: optional(t.Summary), ContentRating: optional(t.ContentRating),
	}
	if out.Genres == nil {
		out.Genres = []string{}
	}
	if t.Year > 0 {
		out.Year = &t.Year
	}
	if t.Minutes > 0 {
		out.Minutes = &t.Minutes
	}
	if t.Episodes > 0 {
		out.Episodes = &t.Episodes
	}
	if t.Rating > 0 {
		r := float32(t.Rating)
		out.Rating = &r
	}
	return out
}

// watchUnavailable answers when Plex is down, not set up or not connected; the cause is logged, not sent.
func watchUnavailable(ctx context.Context, err error) oapi.UnavailableJSONResponse {
	slog.WarnContext(ctx, "watch unavailable", "err", err)
	return oapi.UnavailableJSONResponse(errorBody(ctx, "watch_unavailable", ""))
}

func (h *handlers) SuggestWatch(ctx context.Context, req oapi.SuggestWatchRequestObject) (oapi.SuggestWatchResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	ideas, err := h.watch.Suggest(ctx, u, req.HouseholdId, deref(req.Body.Mood), i18n.FromContext(ctx).String())
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.SuggestWatch404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.SuggestWatch400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_text", err.Error(), "max", domain.MaxWatchMood))}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.SuggestWatch503JSONResponse{UnavailableJSONResponse: ideasUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	out := oapi.SuggestWatch200JSONResponse{Ideas: make([]oapi.WatchIdea, len(ideas))}
	for i, x := range ideas {
		out.Ideas[i] = oapi.WatchIdea{Name: x.Name, Kind: oapi.MediaKind(x.Kind), Reason: x.Reason}
		if x.Year > 0 {
			out.Ideas[i].Year = &x.Year
		}
		if x.Title != nil {
			t := toWatchTitle(*x.Title)
			out.Ideas[i].Title = &t
		}
	}
	return out, nil
}
