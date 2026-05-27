// Package handlers contains HTTP handlers for the GoGrab API.
package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/timhasenkamp/gograb/internal/audit"
	"github.com/timhasenkamp/gograb/internal/auth"
	"github.com/timhasenkamp/gograb/internal/db"
	"github.com/timhasenkamp/gograb/internal/notify"
	"github.com/timhasenkamp/gograb/internal/token"
)

// Branding is operator-configured copy/logo shown on the public submit page.
type Branding struct {
	Name    string `json:"name"`
	LogoURL string `json:"logo_url,omitempty"`
	Color   string `json:"color,omitempty"`
}

// Deps bundles handler dependencies. Constructed once in main.
type Deps struct {
	Queries            db.Querier
	Notifier           notify.Notifier
	Audit              *audit.Logger
	Log                *slog.Logger
	DefaultTTL         time.Duration
	MaxCiphertextBytes int
	Branding           Branding

	// auth is attached by WithAuth once the WebAuthn service is constructed.
	auth *AuthDeps
}

func New(
	q db.Querier,
	n notify.Notifier,
	a *audit.Logger,
	log *slog.Logger,
	defaultTTL time.Duration,
	maxCT int,
) *Deps {
	return &Deps{
		Queries:            q,
		Notifier:           n,
		Audit:              a,
		Log:                log,
		DefaultTTL:         defaultTTL,
		MaxCiphertextBytes: maxCT,
		Branding:           Branding{Name: "GoGrab"},
	}
}

// WithBranding overrides the default branding. Returns d for chaining.
func (d *Deps) WithBranding(b Branding) *Deps {
	if b.Name == "" {
		b.Name = "GoGrab"
	}
	d.Branding = b
	return d
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
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("trailing data in body")
	}
	return nil
}

// getOrCreateOperator looks up the operator row for the Authentik user, or
// creates one on first sight with a fresh 32-byte PRF salt.
func (d *Deps) getOrCreateOperator(ctx context.Context, u auth.User) (db.Operator, error) {
	op, err := d.Queries.GetOperatorByUsername(ctx, u.Username)
	if err == nil {
		return op, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return op, fmt.Errorf("get operator: %w", err)
	}
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return op, fmt.Errorf("gen prf salt: %w", err)
	}
	return d.Queries.CreateOperator(ctx, db.CreateOperatorParams{
		Username: u.Username,
		Email:    u.Email,
		PrfSalt:  salt,
	})
}

// requestSummary is the JSON shape returned for admin endpoints.
type requestSummary struct {
	ID          string          `json:"id"`
	Token       string          `json:"token"`
	Description string          `json:"description"`
	CreatedAt   string          `json:"created_at"`
	ExpiresAt   string          `json:"expires_at"`
	SubmittedAt *string         `json:"submitted_at"`
	RetrievedAt *string         `json:"retrieved_at"`
	Status      string          `json:"status"`
	FormSchema  json.RawMessage `json:"form_schema,omitempty"`
}

func toSummary(r db.Request) requestSummary {
	s := requestSummary{
		ID:          r.ID.String(),
		Token:       r.Token,
		Description: r.Description,
		CreatedAt:   r.CreatedAt.Time.UTC().Format(time.RFC3339),
		ExpiresAt:   r.ExpiresAt.Time.UTC().Format(time.RFC3339),
		Status:      r.Status,
		FormSchema:  r.FormSchema,
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

func effectiveStatus(r db.Request) string {
	if r.Status == "pending" && r.ExpiresAt.Valid && time.Now().After(r.ExpiresAt.Time) {
		return "expired"
	}
	return r.Status
}

// --- admin handlers --------------------------------------------------------

type createRequestBody struct {
	Description    string          `json:"description"`
	ExpiresInHours int             `json:"expires_in_hours"`
	WrappedKeyB64  string          `json:"wrapped_key_b64"`
	WrapIvB64      string          `json:"wrap_iv_b64"`
	FormSchema     json.RawMessage `json:"form_schema"`
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
	op, err := d.getOrCreateOperator(r.Context(), u)
	if err != nil {
		d.Log.Error("operator lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "operator lookup failed")
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
	wrappedKey, err := decodeB64(body.WrappedKeyB64)
	if err != nil || len(wrappedKey) == 0 || len(wrappedKey) > 256 {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid wrapped_key_b64")
		return
	}
	wrapIv, err := decodeB64(body.WrapIvB64)
	if err != nil || len(wrapIv) != 12 {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid wrap_iv_b64 (need 12 bytes)")
		return
	}
	schemaBytes, _, err := ValidateFormSchema(body.FormSchema)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
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
		OperatorID:  op.ID,
		ExpiresAt:   expiresAt,
		WrappedKey:  wrappedKey,
		WrapIv:      wrapIv,
		FormSchema:  schemaBytes,
	})
	if err != nil {
		d.Log.Error("create request", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to create request")
		return
	}

	d.Audit.Log(r.Context(), audit.Entry{
		Actor: "operator:" + u.Username, Action: "request.create",
		RequestID: &req.ID, OperatorID: &op.ID,
		Request: r,
	})

	writeJSON(w, http.StatusCreated, createRequestResponse{
		RequestID: req.ID.String(),
		Token:     req.Token,
	})
}

type listResponse struct {
	Items  []requestSummary `json:"items"`
	Total  int32            `json:"total"`
	Limit  int32            `json:"limit"`
	Offset int32            `json:"offset"`
}

// GET /api/admin/requests?limit=&offset=&q=
func (d *Deps) AdminList(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no user")
		return
	}
	op, err := d.getOrCreateOperator(r.Context(), u)
	if err != nil {
		d.Log.Error("operator lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "operator lookup failed")
		return
	}

	q := r.URL.Query()
	limit := int32(50)
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = int32(n)
		}
	}
	offset := int32(0)
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = int32(n)
		}
	}
	search := strings.TrimSpace(q.Get("q"))
	if len(search) > 100 {
		search = search[:100]
	}

	rows, err := d.Queries.ListRequestsByOperator(r.Context(), db.ListRequestsByOperatorParams{
		OperatorID: op.ID,
		Search:     search,
		Lim:        limit,
		Off:        offset,
	})
	if err != nil {
		d.Log.Error("list requests", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to list")
		return
	}
	total, err := d.Queries.CountRequestsByOperator(r.Context(), db.CountRequestsByOperatorParams{
		OperatorID: op.ID,
		Search:     search,
	})
	if err != nil {
		d.Log.Error("count requests", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to count")
		return
	}
	items := make([]requestSummary, 0, len(rows))
	for _, row := range rows {
		s := toSummary(row)
		s.Status = effectiveStatus(row)
		items = append(items, s)
	}
	writeJSON(w, http.StatusOK, listResponse{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// GET /api/admin/requests/{id}
func (d *Deps) AdminGet(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no user")
		return
	}
	op, err := d.getOrCreateOperator(r.Context(), u)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "operator lookup failed")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
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
	if row.OperatorID != op.ID {
		writeError(w, http.StatusNotFound, "not_found", "request not found")
		return
	}
	s := toSummary(row)
	s.Status = effectiveStatus(row)
	writeJSON(w, http.StatusOK, s)
}

type retrieveResponse struct {
	CiphertextB64 string `json:"ciphertext_b64"`
	IvB64         string `json:"iv_b64"`
	WrappedKeyB64 string `json:"wrapped_key_b64"`
	WrapIvB64     string `json:"wrap_iv_b64"`
}

// POST /api/admin/requests/{id}/retrieve
func (d *Deps) AdminRetrieve(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no user")
		return
	}
	op, err := d.getOrCreateOperator(r.Context(), u)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "operator lookup failed")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
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
		writeError(w, http.StatusInternalServerError, "internal", "failed to load")
		return
	}
	if row.OperatorID != op.ID {
		writeError(w, http.StatusNotFound, "not_found", "request not found")
		return
	}
	if row.Status != "submitted" || len(row.Ciphertext) == 0 || len(row.Iv) == 0 ||
		len(row.WrappedKey) == 0 || len(row.WrapIv) == 0 {
		writeError(w, http.StatusConflict, "not_ready", "no ciphertext to retrieve")
		return
	}

	// Capture all crypto material first; only purge after we know we have it.
	ct := append([]byte(nil), row.Ciphertext...)
	iv := append([]byte(nil), row.Iv...)
	wk := append([]byte(nil), row.WrappedKey...)
	wiv := append([]byte(nil), row.WrapIv...)
	if _, err := d.Queries.MarkRetrievedAndPurge(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "not_ready", "already retrieved")
			return
		}
		d.Log.Error("mark retrieved", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to retrieve")
		return
	}

	d.Audit.Log(r.Context(), audit.Entry{
		Actor: "operator:" + u.Username, Action: "request.retrieve",
		RequestID: &row.ID, OperatorID: &op.ID,
		Request: r,
	})
	d.Notifier.Dispatch(r.Context(), notify.Event{
		Type:        "request.retrieved",
		RequestID:   row.ID.String(),
		Description: row.Description,
		CreatedBy:   u.Username,
		OccurredAt:  time.Now().UTC(),
	})

	writeJSON(w, http.StatusOK, retrieveResponse{
		CiphertextB64: base64.RawURLEncoding.EncodeToString(ct),
		IvB64:         base64.RawURLEncoding.EncodeToString(iv),
		WrappedKeyB64: base64.RawURLEncoding.EncodeToString(wk),
		WrapIvB64:     base64.RawURLEncoding.EncodeToString(wiv),
	})
}

// DELETE /api/admin/requests/{id}
func (d *Deps) AdminDelete(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no user")
		return
	}
	op, err := d.getOrCreateOperator(r.Context(), u)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "operator lookup failed")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	if err := d.Queries.DeleteRequest(r.Context(), db.DeleteRequestParams{
		ID:         id,
		OperatorID: op.ID,
	}); err != nil {
		d.Log.Error("delete request", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to delete")
		return
	}
	d.Audit.Log(r.Context(), audit.Entry{
		Actor: "operator:" + u.Username, Action: "request.delete",
		RequestID: &id, OperatorID: &op.ID,
		Request: r,
	})
	w.WriteHeader(http.StatusNoContent)
}

// --- public handlers -------------------------------------------------------

type publicMeta struct {
	Description string          `json:"description"`
	ExpiresAt   string          `json:"expires_at"`
	Status      string          `json:"status"`
	FormSchema  json.RawMessage `json:"form_schema"`
	Branding    Branding        `json:"branding"`
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
		FormSchema:  row.FormSchema,
		Branding:    d.Branding,
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
			writeError(w, http.StatusConflict, "not_accepting", "request not accepting submissions")
			return
		}
		d.Log.Error("submit ciphertext", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to submit")
		return
	}

	d.Audit.Log(r.Context(), audit.Entry{
		Actor: "customer", Action: "request.submit",
		RequestID: &row.ID, OperatorID: &row.OperatorID,
		Request: r,
	})
	d.Notifier.Dispatch(r.Context(), notify.Event{
		Type:        "request.submitted",
		RequestID:   row.ID.String(),
		Description: row.Description,
		CreatedBy:   "", // operator username not joined here; populated only in retrieved
		OccurredAt:  time.Now().UTC(),
	})

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- small utils -----------------------------------------------------------

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
