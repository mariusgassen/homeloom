package http

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (h *handlers) CreateCalendarProfile(ctx context.Context, req oapi.CreateCalendarProfileRequestObject) (oapi.CreateCalendarProfileResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	server, err := url.Parse(h.publicURL)
	if err != nil || server.Hostname() == "" {
		return nil, fmt.Errorf("calendar profile needs PUBLIC_URL: %q", h.publicURL)
	}
	t, password, err := h.agents.CreateToken(ctx, u, req.HouseholdId, req.Body.Name, domain.AccessCalendar)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.CreateCalendarProfile404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.CreateCalendarProfile400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_text", err.Error(), "max", domain.MaxMCPTokenName))}, nil
	case err != nil:
		return nil, err
	}
	return calendarProfile(appleProfile(server, req.HouseholdId, t.ID, u.Name, password)), nil
}

// calendarProfile is served as a download, so Safari hands it to Settings to install.
type calendarProfile []byte

func (p calendarProfile) VisitCreateCalendarProfileResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Header().Set("Content-Disposition", `attachment; filename="Homeloom.mobileconfig"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, err := io.Copy(w, bytes.NewReader(p))
	return err
}

// appleProfile is an Apple configuration profile with one CalDAV account (payload
// com.apple.caldav.account). It is not signed, so iOS shows it as not verified.
func appleProfile(server *url.URL, householdID, tokenID uuid.UUID, user, password string) []byte {
	port := 443
	if p, err := strconv.Atoi(server.Port()); err == nil {
		port = p
	} else if server.Scheme == "http" {
		port = 80
	}
	ssl := "<false/>"
	if server.Scheme == "https" {
		ssl = "<true/>"
	}
	if user == "" {
		user = "Homeloom"
	}
	id := "homeloom." + tokenID.String()
	// Stable per password, so installing the same profile again replaces it.
	payloadUUID := uuid.NewSHA1(tokenID, []byte("caldav")).String()
	var b bytes.Buffer
	str := func(key, value string) {
		b.WriteString("<key>" + key + "</key><string>")
		_ = xml.EscapeText(&b, []byte(value))
		b.WriteString("</string>")
	}
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>PayloadContent</key><array><dict>`)
	str("CalDAVAccountDescription", "Homeloom")
	str("CalDAVHostName", server.Hostname())
	b.WriteString("<key>CalDAVPort</key><integer>" + strconv.Itoa(port) + "</integer>")
	b.WriteString("<key>CalDAVUseSSL</key>" + ssl)
	str("CalDAVPrincipalURL", "/dav/"+householdID.String()+"/")
	str("CalDAVUsername", user)
	str("CalDAVPassword", password)
	str("PayloadDisplayName", "Homeloom")
	str("PayloadIdentifier", id+".caldav")
	str("PayloadType", "com.apple.caldav.account")
	str("PayloadUUID", payloadUUID)
	b.WriteString("<key>PayloadVersion</key><integer>1</integer></dict></array>")
	str("PayloadDisplayName", "Homeloom")
	str("PayloadIdentifier", id)
	str("PayloadType", "Configuration")
	str("PayloadUUID", tokenID.String())
	b.WriteString("<key>PayloadVersion</key><integer>1</integer></dict></plist>\n")
	return b.Bytes()
}
