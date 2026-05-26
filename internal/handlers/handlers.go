// Package handlers contains HTTP handlers for the GoGrab API.
package handlers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/timhasenkamp/gograb/internal/auth"
	"github.com/timhasenkamp/gograb/internal/db"
	"github.com/timhasenkamp/gograb/internal/notify"
	"github.com/timhasenkamp/gograb/internal/token"
)

// Deps bundles handler dependencies. Constructed once in main.
type Deps struct {
	Queries            db.Querier
	Notifier           notify.Notifier
	Log                *slog.Logger
	DefaultTTL         time.Duration
	MaxCiphertextBytes int
}

// New returns a *Deps with the given configuration.
func New(q db.Querier, n notify.Notifier, log *slog.Logger, defaultTTL time.Duration, maxCT int) *Deps {
	return &Deps{
		Queries:            q,
		Notifier:           n,
		Log:                log,
		DefaultTTL:         defaultTTL,
		MaxCiphertextBytes: maxCT,
	}
}

// --- helpers ---------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": code, "message": msg})
}

func readJSON(r *http.Request, max int64, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, max)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	// reject trailing junk
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("trailing data in body")
	}
	return nil
}

// requestSummary is the JSON shape returned for admin endpoints.
type requestSummary struct {
	ID          string  `json:"id"`
	Token       string  `json:"token"`
	Description string  `json:"description"`
	CreatedAt   string  `json:"created_at"`
	ExpiresAt   string  `json:"expires_at"`
	SubmittedAt *string `json:"submitted_at"`
	RetrievedAt *string `json:"retrieved_at"`
	Status      string  `json:"status"`
}

func toSummary(r db.Request) requestSummary {
	s := requestSummary{
		ID:          r.ID.String(),
		Token:       r.Token,
		Description: r.Description,
		CreatedAt:   r.CreatedAt.Time.UTC().Format(time.RFC3339),
		ExpiresAt:   r.ExpiresAt.Time.UTC().Format(time.RFC3339),
		Status:      r.Status,
	}
	if r.SubmittedAt.Valid {
		t := r.SubmittedAt.Time.UTC().Format(time.RFC3339)
		s.SubmittedAt = &t
	}
	if r.RetrievedAt.Valid {
		t := r.RetrievedAt.Time.UTC().Format(time.RFC3339)
		s.RetrievedAt = &t
	}
	return s
}

// effectiveStatus folds expired-but-still-pending rows into "expired" for
// API responses without rewriting them in the DB. The expiry sweeper handles
// the persistent update.
func effectiveStatus(r db.Request) string {
	if r.Status == "pending" && r.ExpiresAt.Valid && time.Now().After(r.ExpiresAt.Time) {
		return "expired"
	}
	return r.Status
}

// --- admin handlers --------------------------------------------------------

type createRequestBody struct {
	Description    string `json:"description"`
	ExpiresInHours int    `json:"expires_in_hours"`
}

type createRequestResponse struct {
	RequestID string `json:"request_id"`
	Token     string `json:"token"`
}

// POST /api/admin/requests
func (d *Deps) AdminCreate(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no user")
		return
	}
	var body createRequestBody
	if err := readJSON(r, 4096, &body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	body.Description = trimAndCap(body.Description, 200)
	if body.Description == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "description required")
		return
	}
	if body.ExpiresInHours <= 0 || body.ExpiresInHours > 720 {
		writeError(w, http.StatusBadRequest, "bad_request", "expires_in_hours must be 1..720")
		return
	}

	tok, err := token.New()
	if err != nil {
		d.Log.Error("token gen", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "token generation failed")
		return
	}
	expiresAt := pgtype.Timestamptz{
		Time:  time.Now().Add(time.Duration(body.ExpiresInHours) * time.Hour),
		Valid: true,
	}
	req, err := d.Queries.CreateRequest(r.Context(), db.CreateRequestParams{
		Token:       tok,
		Description: body.Description,
		CreatedBy:   u.Username,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		d.Log.Error("create request", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to create request")
		return
	}
	writeJSON(w, http.StatusCreated, createRequestResponse{
		RequestID: req.ID.String(),
		Token:     req.Token,
	})
}

// GET /api/admin/requests
func (d *Deps) AdminList(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no user")
		return
	}
	rows, err := d.Queries.ListRequestsByUser(r.Context(), u.Username)
	if err != nil {
		d.Log.Error("list requests", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to list")
		return
	}
	out := make([]requestSummary, 0, len(rows))
	for _, row := range rows {
		s := toSummary(row)
		s.Status = effectiveStatus(row)
		out = append(out, s)
	}
	writeJSON(w, http.StatusOK, out)
}

// GET /api/admin/requests/{id}
func (d *Deps) AdminGet(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no user")
		return
	}
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	row, err := d.Queries.GetRequestByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "request not found")
			return
		}
		d.Log.Error("get request", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to load")
		return
	}
	if row.CreatedBy != u.Username {
		// don't disclose existence
		writeError(w, http.StatusNotFound, "not_found", "request not found")
		return
	}
	s := toSummary(row)
	s.Status = effectiveStatus(row)
	writeJSON(w, http.StatusOK, s)
}

// POST /api/admin/requests/{id}/retrieve
func (d *Deps) AdminRetrieve(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no user")
		return
	}
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	row, err := d.Queries.GetRequestByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "request not found")
			return
		}
		d.Log.Error("get request", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to load")
		return
	}
	if row.CreatedBy != u.Username {
		writeError(w, http.StatusNotFound, "not_found", "request not found")
		return
	}
	if row.Status != "submitted" || len(row.Ciphertext) == 0 || len(row.Iv) == 0 {
		writeError(w, http.StatusConflict, "not_ready", "no ciphertext to retrieve")
		return
	}

	// Capture ciphertext first, THEN purge. If purge fails we still must not
	// double-deliver, so we abort.
	ct := append([]byte(nil), row.Ciphertext...)
	iv := append([]byte(nil), row.Iv...)
	if _, err := d.Queries.MarkRetrievedAndPurge(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "not_ready", "already retrieved")
			return
		}
		d.Log.Error("mark retrieved", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to retrieve")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"ciphertext_b64": base64.RawURLEncoding.EncodeToString(ct),
		"iv_b64":         base64.RawURLEncoding.EncodeToString(iv),
	})
}

// DELETE /api/admin/requests/{id}
func (d *Deps) AdminDelete(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no user")
		return
	}
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	if err := d.Queries.DeleteRequest(r.Context(), db.DeleteRequestParams{ID: id, CreatedBy: u.Username}); err != nil {
		d.Log.Error("delete request", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to delete")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- public handlers -------------------------------------------------------

type publicMeta struct {
	Description string `json:"description"`
	ExpiresAt   string `json:"expires_at"`
	Status      string `json:"status"`
}

// GET /api/requests/{token}/meta
func (d *Deps) PublicMeta(w http.ResponseWriter, r *http.Request) {
	tok := r.PathValue("token")
	if tok == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "missing token")
		return
	}
	row, err := d.Queries.GetRequestByToken(r.Context(), tok)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "request not found")
			return
		}
		d.Log.Error("get by token", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to load")
		return
	}
	writeJSON(w, http.StatusOK, publicMeta{
		Description: row.Description,
		ExpiresAt:   row.ExpiresAt.Time.UTC().Format(time.RFC3339),
		Status:      effectiveStatus(row),
	})
}

type submitBody struct {
	CiphertextB64 string `json:"ciphertext_b64"`
	IvB64         string `json:"iv_b64"`
}

// POST /api/requests/{token}/submit
func (d *Deps) PublicSubmit(w http.ResponseWriter, r *http.Request) {
	tok := r.PathValue("token")
	if tok == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "missing token")
		return
	}
	var body submitBody
	if err := readJSON(r, int64(d.MaxCiphertextBytes)+1024, &body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	ct, err := decodeB64(body.CiphertextB64)
	if err != nil || len(ct) == 0 || len(ct) > d.MaxCiphertextBytes {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid ciphertext")
		return
	}
	iv, err := decodeB64(body.IvB64)
	if err != nil || len(iv) != 12 {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid iv (need 12 bytes)")
		return
	}

	row, err := d.Queries.SubmitCiphertext(r.Context(), db.SubmitCiphertextParams{
		Token:      tok,
		Ciphertext: ct,
		Iv:         iv,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// either no such token, already submitted, or expired
			writeError(w, http.StatusConflict, "not_accepting", "request not accepting submissions")
			return
		}
		d.Log.Error("submit ciphertext", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to submit")
		return
	}

	d.Notifier.Dispatch(r.Context(), notify.Event{
		Type:        "request.submitted",
		RequestID:   row.ID.String(),
		Description: row.Description,
		CreatedBy:   row.CreatedBy,
		OccurredAt:  time.Now().UTC(),
	})

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- small utils -----------------------------------------------------------

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

func decodeB64(s string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.StdEncoding.DecodeString(s)
}

func trimAndCap(s string, n int) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 {
		c := s[len(s)-1]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		s = s[:len(s)-1]
	}
	if len(s) > n {
		s = s[:n]
	}
	return s
}
