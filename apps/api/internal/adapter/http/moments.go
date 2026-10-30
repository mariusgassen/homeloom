package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// monthLayout is how the contract writes a month.
const monthLayout = "2006-01"

func (h *handlers) ListMoments(ctx context.Context, req oapi.ListMomentsRequestObject) (oapi.ListMomentsResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	var month time.Time
	if req.Params.Month != nil {
		if month, err = time.Parse(monthLayout, *req.Params.Month); err != nil {
			return oapi.ListMoments400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid", err.Error()))}, nil
		}
	}
	mm, err := h.moments.Month(ctx, u, req.HouseholdId, month)
	if errors.Is(err, domain.ErrNotFound) {
		return oapi.ListMoments404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	}
	if err != nil {
		return nil, err
	}
	out := oapi.ListMoments200JSONResponse{Month: mm.Month.Format(monthLayout), Moments: toMoments(mm.Moments)}
	if !mm.Earlier.IsZero() {
		e := mm.Earlier.Format(monthLayout)
		out.Earlier = &e
	}
	return out, nil
}

func (h *handlers) CreateMoment(ctx context.Context, req oapi.CreateMomentRequestObject) (oapi.CreateMomentResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	b := req.Body
	m, err := h.moments.Create(ctx, u, req.HouseholdId, domain.MomentKind(b.Kind), toMomentFields(oapi.MomentFields{
		Text: b.Text, Visibility: b.Visibility, RevealAt: b.RevealAt, Day: b.Day,
	}))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.CreateMoment404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.CreateMoment400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidMoment(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.CreateMoment201JSONResponse(toMoment(m)), nil
}

func (h *handlers) GetResurfaced(ctx context.Context, req oapi.GetResurfacedRequestObject) (oapi.GetResurfacedResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	ms, line, err := h.moments.Resurfaced(ctx, u, req.HouseholdId)
	if errors.Is(err, domain.ErrNotFound) {
		return oapi.GetResurfaced404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	}
	if err != nil {
		return nil, err
	}
	out := oapi.GetResurfaced200JSONResponse{OnThisDay: toMoments(ms)}
	if line != nil {
		m := toMoment(*line)
		out.Gratitude = &m
	}
	return out, nil
}

func (h *handlers) GetMoment(ctx context.Context, req oapi.GetMomentRequestObject) (oapi.GetMomentResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	m, err := h.moments.Get(ctx, u, req.MomentId)
	if errors.Is(err, domain.ErrNotFound) {
		return oapi.GetMoment404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	}
	if err != nil {
		return nil, err
	}
	return oapi.GetMoment200JSONResponse(toMoment(m)), nil
}

func (h *handlers) UpdateMoment(ctx context.Context, req oapi.UpdateMomentRequestObject) (oapi.UpdateMomentResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	m, err := h.moments.Update(ctx, u, req.MomentId, toMomentFields(*req.Body))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UpdateMoment404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return oapi.UpdateMoment403JSONResponse(errorBody(ctx, "not_yours", err.Error())), nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.UpdateMoment400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidMoment(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.UpdateMoment200JSONResponse(toMoment(m)), nil
}

func (h *handlers) DeleteMoment(ctx context.Context, req oapi.DeleteMomentRequestObject) (oapi.DeleteMomentResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.moments.Delete(ctx, u, req.MomentId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteMoment404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return oapi.DeleteMoment403JSONResponse(errorBody(ctx, "not_yours", err.Error())), nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteMoment204Response{}, nil
}

func (h *handlers) GetMomentPhoto(ctx context.Context, req oapi.GetMomentPhotoRequestObject) (oapi.GetMomentPhotoResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	p, err := h.moments.Photo(ctx, u, req.MomentId)
	if errors.Is(err, domain.ErrNotFound) {
		return oapi.GetMomentPhoto404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	}
	if err != nil {
		return nil, err
	}
	// photoUrl names the photo's version, so the browser may keep it; it is private to the viewer.
	cache := "private, max-age=31536000, immutable"
	return oapi.GetMomentPhoto200ImagejpegResponse{
		Body: bytes.NewReader(p.Data), ContentLength: int64(len(p.Data)), Headers: oapi.GetMomentPhoto200ResponseHeaders{CacheControl: &cache},
	}, nil
}

func (h *handlers) SetMomentPhoto(ctx context.Context, req oapi.SetMomentPhotoRequestObject) (oapi.SetMomentPhotoResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	// One byte more than allowed is enough to tell the photo is too big.
	data, err := io.ReadAll(io.LimitReader(req.Body, domain.MaxPhotoBytes+1))
	if err != nil {
		return oapi.SetMomentPhoto400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_photo", err.Error(), "max", domain.MaxPhotoBytes>>20))}, nil
	}
	m, err := h.moments.SetPhoto(ctx, u, req.MomentId, domain.Photo{ContentType: "image/jpeg", Data: data})
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.SetMomentPhoto404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return oapi.SetMomentPhoto403JSONResponse(errorBody(ctx, "not_yours", err.Error())), nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.SetMomentPhoto400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidMoment(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.SetMomentPhoto200JSONResponse(toMoment(m)), nil
}

func (h *handlers) DeleteMomentPhoto(ctx context.Context, req oapi.DeleteMomentPhotoRequestObject) (oapi.DeleteMomentPhotoResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	m, err := h.moments.DeletePhoto(ctx, u, req.MomentId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteMomentPhoto404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return oapi.DeleteMomentPhoto403JSONResponse(errorBody(ctx, "not_yours", err.Error())), nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteMomentPhoto200JSONResponse(toMoment(m)), nil
}

func (h *handlers) GetMomentVideo(ctx context.Context, req oapi.GetMomentVideoRequestObject) (oapi.GetMomentVideoResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.moments.Video(ctx, u, req.MomentId)
	if errors.Is(err, domain.ErrNotFound) {
		return oapi.GetMomentVideo404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	}
	if err != nil {
		return nil, err
	}
	offset, length, partial, ok := byteRange(req.Params.Range, v.Size)
	if !ok {
		return oapi.GetMomentVideo416Response{Headers: oapi.GetMomentVideo416ResponseHeaders{ContentRange: fmt.Sprintf("bytes */%d", v.Size)}}, nil
	}
	body, err := v.Open(ctx, offset, length)
	if err != nil {
		return nil, err
	}
	// videoUrl names the video's version, so the browser may keep it; it is private to the viewer.
	cache, ranges := "private, max-age=31536000, immutable", "bytes"
	if !partial {
		return videoResponse{ctx: ctx, visit: oapi.GetMomentVideo200VideoResponse{
			Body: body, ContentType: v.ContentType, ContentLength: length,
			Headers: oapi.GetMomentVideo200ResponseHeaders{AcceptRanges: &ranges, CacheControl: &cache},
		}.VisitGetMomentVideoResponse}, nil
	}
	return videoResponse{ctx: ctx, visit: oapi.GetMomentVideo206VideoResponse{
		Body: body, ContentType: v.ContentType, ContentLength: length,
		Headers: oapi.GetMomentVideo206ResponseHeaders{
			AcceptRanges: &ranges, CacheControl: &cache, ContentRange: fmt.Sprintf("bytes %d-%d/%d", offset, offset+length-1, v.Size),
		},
	}.VisitGetMomentVideoResponse}, nil
}

// videoResponse streams a video. Players drop a stream whenever they seek, so a stream cut off
// because the viewer went away is no failure.
type videoResponse struct {
	ctx   context.Context
	visit func(w http.ResponseWriter) error
}

func (r videoResponse) VisitGetMomentVideoResponse(w http.ResponseWriter) error {
	if err := r.visit(w); err != nil && r.ctx.Err() == nil {
		return err
	}
	return nil
}

// byteRange reads a Range header for a video of size bytes: the part to send and whether it is
// one range (206) rather than the whole video. Several ranges, or a header it can't read, get the
// whole video, as RFC 9110 allows; ok is false for a range that lies outside the video.
func byteRange(header *string, size int64) (offset, length int64, partial, ok bool) {
	whole := func() (int64, int64, bool, bool) { return 0, size, false, true }
	if header == nil {
		return whole()
	}
	spec, found := strings.CutPrefix(strings.TrimSpace(*header), "bytes=")
	if !found || strings.Contains(spec, ",") {
		return whole()
	}
	first, last, found := strings.Cut(strings.TrimSpace(spec), "-")
	if !found {
		return whole()
	}
	if first == "" {
		// The last n bytes.
		n, err := strconv.ParseInt(last, 10, 64)
		if err != nil || n < 0 {
			return whole()
		}
		if n == 0 || size == 0 {
			return 0, 0, false, false
		}
		n = min(n, size)
		return size - n, n, true, true
	}
	from, err := strconv.ParseInt(first, 10, 64)
	if err != nil || from < 0 {
		return whole()
	}
	if from >= size {
		return 0, 0, false, false
	}
	to := size - 1
	if last != "" {
		if to, err = strconv.ParseInt(last, 10, 64); err != nil || to < from {
			return whole()
		}
		to = min(to, size-1)
	}
	return from, to - from + 1, true, true
}

func (h *handlers) SetMomentVideo(ctx context.Context, req oapi.SetMomentVideoRequestObject) (oapi.SetMomentVideoResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	// The type comes without parameters, such as "video/mp4; codecs=…", for the core to check.
	contentType, _, _ := strings.Cut(req.ContentType, ";")
	m, err := h.moments.SetVideo(ctx, u, req.MomentId, strings.ToLower(strings.TrimSpace(contentType)), req.Body)
	var tooBig *http.MaxBytesError
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.SetMomentVideo404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return oapi.SetMomentVideo403JSONResponse(errorBody(ctx, "not_yours", err.Error())), nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.SetMomentVideo503JSONResponse{UnavailableJSONResponse: oapi.UnavailableJSONResponse(errorBody(ctx, "videos_unavailable", err.Error()))}, nil
	case errors.Is(err, domain.ErrInvalid), errors.As(err, &tooBig):
		return oapi.SetMomentVideo400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_video", err.Error(), "max", domain.MaxVideoBytes>>20))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.SetMomentVideo200JSONResponse(toMoment(m)), nil
}

func (h *handlers) DeleteMomentVideo(ctx context.Context, req oapi.DeleteMomentVideoRequestObject) (oapi.DeleteMomentVideoResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	m, err := h.moments.DeleteVideo(ctx, u, req.MomentId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteMomentVideo404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return oapi.DeleteMomentVideo403JSONResponse(errorBody(ctx, "not_yours", err.Error())), nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteMomentVideo200JSONResponse(toMoment(m)), nil
}

// invalidMoment names what is wrong with a moment, so the app can say it in the user's language.
func invalidMoment(ctx context.Context, err error) oapi.Error {
	var te domain.TextError
	switch {
	case errors.As(err, &te):
		return errorBody(ctx, "invalid_text", err.Error(), "max", te.Max)
	case errors.Is(err, domain.ErrRevealAt):
		return errorBody(ctx, "invalid_reveal", err.Error())
	case errors.Is(err, domain.ErrMomentDay):
		return errorBody(ctx, "invalid_moment_day", err.Error())
	case errors.Is(err, domain.ErrPhoto):
		return errorBody(ctx, "invalid_photo", err.Error(), "max", domain.MaxPhotoBytes>>20)
	}
	return errorBody(ctx, "invalid_entry", err.Error())
}

func toMomentFields(b oapi.MomentFields) domain.MomentFields {
	f := domain.MomentFields{Visibility: domain.Visibility(b.Visibility), RevealAt: b.RevealAt, Text: b.Text}
	if b.Day != nil {
		f.Day = b.Day.Time
	}
	return f
}

func toMoments(ms []domain.Moment) []oapi.Moment {
	out := make([]oapi.Moment, len(ms))
	for i, m := range ms {
		out[i] = toMoment(m)
	}
	return out
}

func toMoment(m domain.Moment) oapi.Moment {
	out := oapi.Moment{
		Id: m.ID, Kind: oapi.MomentKind(m.Kind), AuthorId: m.AuthorID, Day: openapi_types.Date{Time: m.Day},
		Visibility: oapi.Visibility(m.Visibility), RevealAt: m.RevealAt, Sealed: m.Sealed, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
	if !m.Sealed {
		out.Text = &m.Text
		if m.PhotoAt != nil {
			url := fmt.Sprintf("/v1/moments/%s/photo?v=%d", m.ID, m.PhotoAt.UnixMilli())
			out.PhotoUrl = &url
		}
		if m.VideoAt != nil {
			url := fmt.Sprintf("/v1/moments/%s/video?v=%d", m.ID, m.VideoAt.UnixMilli())
			out.VideoUrl = &url
		}
	}
	return out
}
