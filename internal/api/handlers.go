package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/honeybbq/tsdns/core"
)

var (
	errDomainRequired    = errors.New("domain is required")
	errInvalidPort       = errors.New("port must be in [0, 65535]")
	errInvalidInstanceID = errors.New("instance_id must be >= 0")
)

// handleHealthz responds with a simple plain text "ok" to indicate the server is running.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// upsertRecordRequest defines the JSON structure for creating or updating a DNS record.
type upsertRecordRequest struct {
	Domain     string   `json:"domain"`
	Targets    []string `json:"targets"`
	InstanceID int64    `json:"instance_id"`
	Port       int32    `json:"port"`
}

// handleRecordsList returns a list of all active DNS records.
func (s *Server) handleRecordsList(w http.ResponseWriter, r *http.Request) {
	records, err := s.record.ListRecords(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}
	writeJSON(w, http.StatusOK, records)
}

// handleRecordsUpsert creates or updates a DNS record.
func (s *Server) handleRecordsUpsert(w http.ResponseWriter, r *http.Request) {
	const maxBodySize = 1 << 20
	req, err := decodeJSON[upsertRecordRequest](w, r, maxBodySize)
	if err != nil {
		return
	}

	errValidate := validateUpsertRecordRequest(req)
	if errValidate != nil {
		writeError(w, http.StatusBadRequest, errValidate.Error())

		return
	}

	var targets []netip.AddrPort
	for _, t := range req.Targets {
		if strings.EqualFold(t, "NORESPONSE") {
			continue
		}

		// Try parsing as AddrPort first
		tp, errParse := netip.ParseAddrPort(t)
		if errParse == nil {
			targets = append(targets, tp)
		} else {
			// Fallback to Addr + req.Port or default 0
			addr, errParseAddr := netip.ParseAddr(strings.Trim(t, "[] "))
			if errParseAddr != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid target %q: %v", t, errParseAddr))

				return
			}

			// Safe conversion because req.Port is validated to be in [0, 65535].
			/* #nosec G115 */
			port := uint16(req.Port)
			targets = append(targets, netip.AddrPortFrom(addr, port))
		}
	}

	rec := &tsdns.Record{
		InstanceID: req.InstanceID,
		Domain:     req.Domain,
		Targets:    targets,
	}

	errUpsert := s.record.UpsertRecord(r.Context(), rec)
	if errUpsert != nil {
		writeError(w, http.StatusInternalServerError, errUpsert.Error())

		return
	}

	// Return the stored record.
	stored, err := s.record.GetRecord(r.Context(), req.Domain)
	if err != nil {
		writeJSON(w, http.StatusCreated, rec)

		return
	}
	writeJSON(w, http.StatusCreated, stored)
}

// handleRecordGet retrieves a single DNS record by domain name.
func (s *Server) handleRecordGet(w http.ResponseWriter, r *http.Request) {
	domain := strings.TrimSpace(r.PathValue("domain"))
	if domain == "" || strings.Contains(domain, "/") {
		writeError(w, http.StatusBadRequest, "invalid domain")

		return
	}

	rec, err := s.record.GetRecord(r.Context(), domain)
	if err != nil {
		writeError(w, http.StatusNotFound, "record not found")

		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// handleRecordDelete removes a single DNS record by domain name.
func (s *Server) handleRecordDelete(w http.ResponseWriter, r *http.Request) {
	domain := strings.TrimSpace(r.PathValue("domain"))
	if domain == "" || strings.Contains(domain, "/") {
		writeError(w, http.StatusBadRequest, "invalid domain")

		return
	}

	err := s.record.DeleteRecord(r.Context(), domain)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRecordPathInvalid catches requests to invalid record paths.
func (s *Server) handleRecordPathInvalid(w http.ResponseWriter, _ *http.Request) {
	// This catches paths like /api/v1/records/foo/bar (including cases where the client sent foo%2Fbar).
	writeError(w, http.StatusBadRequest, "invalid domain")
}

// handleInstanceRecordsDelete removes all DNS records associated with an instance ID.
func (s *Server) handleInstanceRecordsDelete(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSpace(r.PathValue("id"))
	instanceID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || instanceID < 0 {
		writeError(w, http.StatusBadRequest, "invalid instance id")

		return
	}

	err = s.record.DeleteInstanceRecords(r.Context(), instanceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validateUpsertRecordRequest validates the fields of a record upsert request.
func validateUpsertRecordRequest(r upsertRecordRequest) error {
	if strings.TrimSpace(r.Domain) == "" {
		return errDomainRequired
	}
	if r.Port < 0 || r.Port > 65535 {
		return errInvalidPort
	}
	if r.InstanceID < 0 {
		return errInvalidInstanceID
	}

	return nil
}

// decodeJSON decodes a JSON request body into the specified type.
func decodeJSON[T any](w http.ResponseWriter, r *http.Request, maxBytes int64) (T, error) {
	var zero T
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	defer func() { _ = r.Body.Close() }()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var v T
	err := dec.Decode(&v)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")

		return zero, err
	}

	return v, nil
}

// writeJSON writes a JSON response with the specified status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	err := json.NewEncoder(w).Encode(v)
	if err != nil {
		// Log error if encoding fails
		slog.Error("failed to encode json response", slog.Any("error", err))
	}
}

// writeError writes a JSON error response with the specified status code and message.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
