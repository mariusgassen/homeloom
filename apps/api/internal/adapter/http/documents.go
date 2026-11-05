package http

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (h *handlers) ListDocuments(ctx context.Context, req oapi.ListDocumentsRequestObject) (oapi.ListDocumentsResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	ds, err := h.documents.List(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ListDocuments404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	out := make(oapi.ListDocuments200JSONResponse, len(ds))
	for i, d := range ds {
		out[i] = toDocument(d, u.ID)
	}
	return out, nil
}

func (h *handlers) AddDocument(ctx context.Context, req oapi.AddDocumentRequestObject) (oapi.AddDocumentResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	// The type comes without parameters, such as "text/plain; charset=utf-8", for the core to check.
	contentType, _, _ := strings.Cut(req.ContentType, ";")
	d, err := h.documents.Add(ctx, u, req.HouseholdId, req.Params.Name, strings.ToLower(strings.TrimSpace(contentType)), req.Body)
	var tooBig *http.MaxBytesError
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.AddDocument404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.AddDocument503JSONResponse{UnavailableJSONResponse: oapi.UnavailableJSONResponse(errorBody(ctx, "documents_unavailable", err.Error()))}, nil
	case errors.Is(err, domain.ErrInvalid), errors.As(err, &tooBig):
		return oapi.AddDocument400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidDocument(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.AddDocument201JSONResponse(toDocument(d, u.ID)), nil
}

func (h *handlers) UpdateDocument(ctx context.Context, req oapi.UpdateDocumentRequestObject) (oapi.UpdateDocumentResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	name := ""
	if req.Body.Name != nil {
		if name = strings.TrimSpace(*req.Body.Name); name == "" {
			return oapi.UpdateDocument400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidDocument(ctx, domain.TextError{Max: domain.MaxDocumentName}))}, nil
		}
	}
	d, err := h.documents.Update(ctx, u, req.HouseholdId, req.DocumentId, name, req.Body.Shared)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UpdateDocument404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return oapi.UpdateDocument403JSONResponse(errorBody(ctx, "not_yours", err.Error())), nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.UpdateDocument400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidDocument(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.UpdateDocument200JSONResponse(toDocument(d, u.ID)), nil
}

func (h *handlers) DeleteDocument(ctx context.Context, req oapi.DeleteDocumentRequestObject) (oapi.DeleteDocumentResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.documents.Delete(ctx, u, req.HouseholdId, req.DocumentId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteDocument404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return oapi.DeleteDocument403JSONResponse(errorBody(ctx, "not_yours", err.Error())), nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteDocument204Response{}, nil
}

func (h *handlers) GetDocumentFile(ctx context.Context, req oapi.GetDocumentFileRequestObject) (oapi.GetDocumentFileResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	d, f, err := h.documents.File(ctx, u, req.HouseholdId, req.DocumentId)
	if errors.Is(err, domain.ErrNotFound) {
		return oapi.GetDocumentFile404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	}
	if err != nil {
		return nil, err
	}
	body, err := f.Open(ctx, 0, f.Size)
	if err != nil {
		return nil, err
	}
	// Only types a browser shows safely open in place; the rest download. The file is private to
	// the viewer, so no shared cache may keep it, and the browser may not guess another type.
	disposition := "attachment"
	if d.ContentType == "application/pdf" || strings.HasPrefix(d.ContentType, "image/") {
		disposition = "inline"
	}
	disposition = mime.FormatMediaType(disposition, map[string]string{"filename": d.Name})
	cache, nosniff := "private, no-store", "nosniff"
	return fileResponse{ctx: ctx, visit: oapi.GetDocumentFile200AsteriskResponse{
		Body: body, ContentType: d.ContentType, ContentLength: f.Size,
		Headers: oapi.GetDocumentFile200ResponseHeaders{CacheControl: &cache, ContentDisposition: &disposition, XContentTypeOptions: &nosniff},
	}.VisitGetDocumentFileResponse}, nil
}

// fileResponse streams a file; one cut off because the viewer went away is no failure.
type fileResponse struct {
	ctx   context.Context
	visit func(w http.ResponseWriter) error
}

func (r fileResponse) VisitGetDocumentFileResponse(w http.ResponseWriter) error {
	if err := r.visit(w); err != nil && r.ctx.Err() == nil {
		return err
	}
	return nil
}

func invalidDocument(ctx context.Context, err error) oapi.Error {
	var te domain.TextError
	if errors.As(err, &te) {
		return errorBody(ctx, "invalid_text", err.Error(), "max", te.Max)
	}
	return errorBody(ctx, "invalid_document", err.Error(), "max", domain.MaxDocumentBytes>>20)
}

func toDocument(d domain.Document, viewer uuid.UUID) oapi.Document {
	return oapi.Document{
		Id: d.ID, OwnerId: d.OwnerID, Mine: d.OwnerID == viewer, Name: d.Name,
		ContentType: d.ContentType, Size: d.Size, Shared: d.Shared, CreatedAt: d.CreatedAt,
	}
}
