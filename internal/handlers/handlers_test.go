package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/timhasenkamp/gograb/internal/audit"
	"github.com/timhasenkamp/gograb/internal/auth"
	"github.com/timhasenkamp/gograb/internal/db"
	"github.com/timhasenkamp/gograb/internal/notify"
)

// fakeDB is a minimal in-memory implementation of db.Querier — only the
// methods used in handler lifecycle tests are real. Calls to any other
// method panic via the embedded nil interface, which catches accidental
// usage drift in future test changes.
type fakeDB struct {
	db.Querier

	mu              sync.Mutex
	operators       map[string]db.Operator
	requestsByID    map[uuid.UUID]db.Request
	requestsByToken map[string]db.Request
	audits          int
}

func newFakeDB() *fakeDB {
	return &fakeDB{
		operators:       make(map[string]db.Operator),
		requestsByID:    make(map[uuid.UUID]db.Request),
		requestsByToken: make(map[string]db.Request),
	}
}

func (f *fakeDB) GetOperatorByUsername(_ context.Context, username string) (db.Operator, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	op, ok := f.operators[username]
	if !ok {
		return db.Operator{}, pgx.ErrNoRows
	}
	return op, nil
}

func (f *fakeDB) CreateOperator(_ context.Context, arg db.CreateOperatorParams) (db.Operator, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	op := db.Operator{
		ID:        uuid.New(),
		Username:  arg.Username,
		Email:     arg.Email,
		PrfSalt:   arg.PrfSalt,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	f.operators[arg.Username] = op
	return op, nil
}

func (f *fakeDB) CreateRequest(_ context.Context, arg db.CreateRequestParams) (db.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := db.Request{
		ID:          uuid.New(),
		Token:       arg.Token,
		Description: arg.Description,
		OperatorID:  arg.OperatorID,
		CreatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
		ExpiresAt:   arg.ExpiresAt,
		WrappedKey:  arg.WrappedKey,
		WrapIv:      arg.WrapIv,
		FormSchema:  arg.FormSchema,
		Status:      "pending",
	}
	f.requestsByID[r.ID] = r
	f.requestsByToken[r.Token] = r
	return r, nil
}

func (f *fakeDB) GetRequestByID(_ context.Context, id uuid.UUID) (db.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.requestsByID[id]
	if !ok {
		return db.Request{}, pgx.ErrNoRows
	}
	return r, nil
}

func (f *fakeDB) GetRequestByToken(_ context.Context, token string) (db.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.requestsByToken[token]
	if !ok {
		return db.Request{}, pgx.ErrNoRows
	}
	return r, nil
}

func (f *fakeDB) ListRequestsByOperator(_ context.Context, arg db.ListRequestsByOperatorParams) ([]db.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []db.Request
	for _, r := range f.requestsByID {
		if r.OperatorID != arg.OperatorID {
			continue
		}
		if arg.Search != "" && !contains(r.Description, arg.Search) {
			continue
		}
		out = append(out, r)
	}
	// trivial offset+limit
	if int(arg.Off) >= len(out) {
		return nil, nil
	}
	out = out[arg.Off:]
	if int(arg.Lim) < len(out) {
		out = out[:arg.Lim]
	}
	return out, nil
}

func (f *fakeDB) CountRequestsByOperator(_ context.Context, arg db.CountRequestsByOperatorParams) (int32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int32
	for _, r := range f.requestsByID {
		if r.OperatorID != arg.OperatorID {
			continue
		}
		if arg.Search != "" && !contains(r.Description, arg.Search) {
			continue
		}
		n++
	}
	return n, nil
}

// case-insensitive substring (mirrors postgres ILIKE).
func contains(hay, needle string) bool {
	if needle == "" {
		return true
	}
	h, n := []rune(hay), []rune(needle)
	for i := 0; i+len(n) <= len(h); i++ {
		match := true
		for j := range n {
			a, b := h[i+j], n[j]
			if 'A' <= a && a <= 'Z' {
				a += 'a' - 'A'
			}
			if 'A' <= b && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func (f *fakeDB) SubmitCiphertext(_ context.Context, arg db.SubmitCiphertextParams) (db.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.requestsByToken[arg.Token]
	if !ok {
		return db.Request{}, pgx.ErrNoRows
	}
	// SQL constraints: only pending + not yet expired
	if r.Status != "pending" {
		return db.Request{}, pgx.ErrNoRows
	}
	if r.ExpiresAt.Valid && time.Now().After(r.ExpiresAt.Time) {
		return db.Request{}, pgx.ErrNoRows
	}
	r.Status = "submitted"
	r.Ciphertext = arg.Ciphertext
	r.Iv = arg.Iv
	r.SubmittedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	f.requestsByID[r.ID] = r
	f.requestsByToken[r.Token] = r
	return r, nil
}

func (f *fakeDB) MarkRetrievedAndPurge(_ context.Context, id uuid.UUID) (db.MarkRetrievedAndPurgeRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.requestsByID[id]
	if !ok || r.Status != "submitted" {
		return db.MarkRetrievedAndPurgeRow{}, pgx.ErrNoRows
	}
	r.Status = "retrieved"
	r.Ciphertext = nil
	r.Iv = nil
	r.WrappedKey = nil
	r.WrapIv = nil
	r.RetrievedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	f.requestsByID[r.ID] = r
	f.requestsByToken[r.Token] = r
	return db.MarkRetrievedAndPurgeRow{
		ID:          r.ID,
		Token:       r.Token,
		Description: r.Description,
		OperatorID:  r.OperatorID,
		CreatedAt:   r.CreatedAt,
		ExpiresAt:   r.ExpiresAt,
		SubmittedAt: r.SubmittedAt,
		RetrievedAt: r.RetrievedAt,
		Status:      r.Status,
	}, nil
}

func (f *fakeDB) DeleteRequest(_ context.Context, arg db.DeleteRequestParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.requestsByID[arg.ID]
	if !ok || r.OperatorID != arg.OperatorID {
		return nil // matches SQL DELETE-no-match behavior
	}
	delete(f.requestsByID, arg.ID)
	delete(f.requestsByToken, r.Token)
	return nil
}

func (f *fakeDB) InsertAuditLog(_ context.Context, _ db.InsertAuditLogParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audits++
	return nil
}

func (f *fakeDB) CountViewsByRequest(_ context.Context, _ *uuid.UUID) (int32, error) {
	// not modelled — tests that exercise view counts can mock more deeply.
	return 0, nil
}

// --- test setup helpers ---

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestDeps() (*Deps, *fakeDB) {
	fdb := newFakeDB()
	log := discardLogger()
	auditLog := audit.New(fdb, log)
	deps := New(fdb, notify.Nop{}, auditLog, log, 72*time.Hour, 65536)
	return deps, fdb
}

func asUser(req *http.Request, username string) *http.Request {
	ctx := auth.WithUser(req.Context(), auth.User{Username: username, Email: username + "@test"})
	return req.WithContext(ctx)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func validCreateBody(t *testing.T) []byte {
	t.Helper()
	body := map[string]any{
		"description":      "WLAN-Passwort",
		"expires_in_hours": 24,
		"wrapped_key_b64":  b64(bytes.Repeat([]byte{0xAB}, 48)),
		"wrap_iv_b64":      b64(bytes.Repeat([]byte{0xCD}, 12)),
	}
	out, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}

// --- the tests ---

func TestAdminCreate_HappyPath(t *testing.T) {
	deps, fdb := newTestDeps()
	req := httptest.NewRequest("POST", "/api/admin/requests", bytes.NewReader(validCreateBody(t)))
	req = asUser(req, "alice")
	rec := httptest.NewRecorder()
	deps.AdminCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var resp createRequestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Token == "" || resp.RequestID == "" {
		t.Errorf("missing fields in response: %+v", resp)
	}
	if len(fdb.requestsByID) != 1 {
		t.Errorf("expected 1 row stored, got %d", len(fdb.requestsByID))
	}
}

func TestAdminCreate_RejectsWithoutUser(t *testing.T) {
	deps, _ := newTestDeps()
	req := httptest.NewRequest("POST", "/api/admin/requests", bytes.NewReader(validCreateBody(t)))
	rec := httptest.NewRecorder()
	deps.AdminCreate(rec, req) // no auth.WithUser

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAdminCreate_RejectsBadInputs(t *testing.T) {
	cases := []struct {
		name string
		mut  func(m map[string]any)
	}{
		{"empty description", func(m map[string]any) { m["description"] = "   " }},
		{"expiry zero", func(m map[string]any) { m["expires_in_hours"] = 0 }},
		{"expiry too long", func(m map[string]any) { m["expires_in_hours"] = 9999 }},
		{"wrap iv wrong size", func(m map[string]any) { m["wrap_iv_b64"] = b64([]byte{1, 2, 3}) }},
		{"wrapped key missing", func(m map[string]any) { m["wrapped_key_b64"] = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps, _ := newTestDeps()
			var body map[string]any
			_ = json.Unmarshal(validCreateBody(t), &body)
			tc.mut(body)
			raw, _ := json.Marshal(body)
			req := asUser(httptest.NewRequest("POST", "/", bytes.NewReader(raw)), "alice")
			rec := httptest.NewRecorder()
			deps.AdminCreate(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAdminList_FiltersByOperator(t *testing.T) {
	deps, _ := newTestDeps()
	// alice creates one
	rec := httptest.NewRecorder()
	deps.AdminCreate(rec, asUser(httptest.NewRequest("POST", "/", bytes.NewReader(validCreateBody(t))), "alice"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("alice create failed: %d %s", rec.Code, rec.Body.String())
	}
	// bob creates one
	rec = httptest.NewRecorder()
	deps.AdminCreate(rec, asUser(httptest.NewRequest("POST", "/", bytes.NewReader(validCreateBody(t))), "bob"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("bob create failed: %d %s", rec.Code, rec.Body.String())
	}

	// alice lists — sees only her own
	rec = httptest.NewRecorder()
	deps.AdminList(rec, asUser(httptest.NewRequest("GET", "/", nil), "alice"))
	var list listResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Items) != 1 || list.Total != 1 {
		t.Errorf("alice should see 1 request, got items=%d total=%d", len(list.Items), list.Total)
	}
}

func TestAdminList_SearchAndPagination(t *testing.T) {
	deps, fdb := newTestDeps()
	// Seed three requests for alice with different descriptions
	op, _ := fdb.CreateOperator(context.Background(), db.CreateOperatorParams{Username: "alice"})
	descs := []string{"WLAN-Passwort Müller", "Server-Login Acme", "WLAN-Passwort Schmidt"}
	for _, d := range descs {
		_, _ = fdb.CreateRequest(context.Background(), db.CreateRequestParams{
			Token: d, Description: d, OperatorID: op.ID,
			ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
		})
	}

	// Search ?q=wlan → 2 hits
	rec := httptest.NewRecorder()
	deps.AdminList(rec, asUser(httptest.NewRequest("GET", "/?q=wlan", nil), "alice"))
	var got listResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 || len(got.Items) != 2 {
		t.Errorf("search='wlan' → items=%d total=%d, want 2/2", len(got.Items), got.Total)
	}

	// Pagination ?limit=1 → 1 item, total still 3
	rec = httptest.NewRecorder()
	deps.AdminList(rec, asUser(httptest.NewRequest("GET", "/?limit=1", nil), "alice"))
	got = listResponse{}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Total != 3 || len(got.Items) != 1 {
		t.Errorf("limit=1 → items=%d total=%d, want 1/3", len(got.Items), got.Total)
	}
}

func TestAdminGet_ForeignOperatorReturns404(t *testing.T) {
	deps, _ := newTestDeps()
	// alice creates
	rec := httptest.NewRecorder()
	deps.AdminCreate(rec, asUser(httptest.NewRequest("POST", "/", bytes.NewReader(validCreateBody(t))), "alice"))
	var resp createRequestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	// bob tries to read alice's request — must not see it exists
	req := asUser(httptest.NewRequest("GET", "/", nil), "bob")
	req.SetPathValue("id", resp.RequestID)
	rec = httptest.NewRecorder()
	deps.AdminGet(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (info-leak prevention)", rec.Code)
	}
}

func TestSubmitAndRetrieveLifecycle(t *testing.T) {
	deps, _ := newTestDeps()

	// 1. alice creates
	rec := httptest.NewRecorder()
	deps.AdminCreate(rec, asUser(httptest.NewRequest("POST", "/", bytes.NewReader(validCreateBody(t))), "alice"))
	var created createRequestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	// 2. customer fetches meta (anonymous)
	req := httptest.NewRequest("GET", "/", nil)
	req.SetPathValue("token", created.Token)
	rec = httptest.NewRecorder()
	deps.PublicMeta(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("meta status = %d", rec.Code)
	}

	// 3. customer submits
	submitBody, _ := json.Marshal(map[string]string{
		"ciphertext_b64": b64(bytes.Repeat([]byte{0x11}, 64)),
		"iv_b64":         b64(bytes.Repeat([]byte{0x22}, 12)),
	})
	req = httptest.NewRequest("POST", "/", bytes.NewReader(submitBody))
	req.SetPathValue("token", created.Token)
	rec = httptest.NewRecorder()
	deps.PublicSubmit(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit status = %d body=%s", rec.Code, rec.Body.String())
	}

	// 4. second submit should fail (status transitioned away from pending)
	req = httptest.NewRequest("POST", "/", bytes.NewReader(submitBody))
	req.SetPathValue("token", created.Token)
	rec = httptest.NewRecorder()
	deps.PublicSubmit(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("second submit status = %d, want 409", rec.Code)
	}

	// 5. alice retrieves
	req = asUser(httptest.NewRequest("POST", "/", nil), "alice")
	req.SetPathValue("id", created.RequestID)
	rec = httptest.NewRecorder()
	deps.AdminRetrieve(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("retrieve status = %d body=%s", rec.Code, rec.Body.String())
	}
	var retrieved retrieveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &retrieved); err != nil {
		t.Fatal(err)
	}
	if retrieved.CiphertextB64 == "" || retrieved.WrappedKeyB64 == "" {
		t.Errorf("retrieve missing material: %+v", retrieved)
	}

	// 6. second retrieve fails (purged)
	req = asUser(httptest.NewRequest("POST", "/", nil), "alice")
	req.SetPathValue("id", created.RequestID)
	rec = httptest.NewRecorder()
	deps.AdminRetrieve(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("second retrieve status = %d, want 409 (already retrieved)", rec.Code)
	}
}

func TestPublicSubmit_RejectsBadIv(t *testing.T) {
	deps, _ := newTestDeps()
	// alice creates so the token exists
	rec := httptest.NewRecorder()
	deps.AdminCreate(rec, asUser(httptest.NewRequest("POST", "/", bytes.NewReader(validCreateBody(t))), "alice"))
	var created createRequestResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	bad, _ := json.Marshal(map[string]string{
		"ciphertext_b64": b64(bytes.Repeat([]byte{0x11}, 32)),
		"iv_b64":         b64([]byte{0x01, 0x02, 0x03}), // not 12 bytes
	})
	req := httptest.NewRequest("POST", "/", bytes.NewReader(bad))
	req.SetPathValue("token", created.Token)
	rec = httptest.NewRecorder()
	deps.PublicSubmit(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestPublicMeta_UnknownToken_Returns404(t *testing.T) {
	deps, _ := newTestDeps()
	req := httptest.NewRequest("GET", "/", nil)
	req.SetPathValue("token", "nonexistent")
	rec := httptest.NewRecorder()
	deps.PublicMeta(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

