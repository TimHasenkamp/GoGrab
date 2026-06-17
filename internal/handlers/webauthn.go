package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/timhasenkamp/gograb/internal/audit"
	"github.com/timhasenkamp/gograb/internal/auth"
	"github.com/timhasenkamp/gograb/internal/db"
	gogwebauthn "github.com/timhasenkamp/gograb/internal/webauthn"

	"github.com/go-webauthn/webauthn/protocol"
	gowebauthn "github.com/go-webauthn/webauthn/webauthn"
)

// AuthDeps extends Deps with the WebAuthn service. We split the construction
// to keep Deps focused; main.go calls handlers.New then handlers.WithAuth.
type AuthDeps struct {
	WebAuthn *gogwebauthn.Service
}

// WithAuth attaches WebAuthn helpers. Returns d for chaining.
func (d *Deps) WithAuth(a *AuthDeps) *Deps {
	d.auth = a
	return d
}

// auth is added to Deps via WithAuth (declared here to keep handlers.go diff
// small in this commit). We add the field via a small unexported declaration
// adjacent so that the package still compiles cleanly.
//
// (See deps_auth_field.go)

// --- shared bits -----------------------------------------------------------

type authStatusResponse struct {
	HasCredentials bool   `json:"has_credentials"`
	PRFSaltB64     string `json:"prf_salt_b64"`
	Username       string `json:"username"`
}

// GET /api/admin/auth/status
func (d *Deps) AuthStatus(w http.ResponseWriter, r *http.Request) {
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
	n, err := d.Queries.CountCredentialsByOperator(r.Context(), op.ID)
	if err != nil {
		d.Log.Error("count credentials", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "count failed")
		return
	}
	writeJSON(w, http.StatusOK, authStatusResponse{
		HasCredentials: n > 0,
		PRFSaltB64:     base64.RawURLEncoding.EncodeToString(op.PrfSalt),
		Username:       u.Username,
	})
}

// --- registration ---------------------------------------------------------

type registerBeginResponse struct {
	Options      json.RawMessage `json:"options"`
	SessionToken string          `json:"session_token"`
	PRFSaltB64   string          `json:"prf_salt_b64"`
}

// POST /api/admin/auth/register/begin
func (d *Deps) AuthRegisterBegin(w http.ResponseWriter, r *http.Request) {
	if d.auth == nil {
		writeError(w, http.StatusInternalServerError, "internal", "webauthn not configured")
		return
	}
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
	creds, err := d.Queries.ListCredentialsByOperator(r.Context(), op.ID)
	if err != nil {
		d.Log.Error("list creds", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "list failed")
		return
	}
	user := gogwebauthn.NewOperator(op, creds)

	// Exclude already-registered credentials so the authenticator doesn't
	// silently re-create the same one.
	excludeList := make([]protocol.CredentialDescriptor, 0, len(creds))
	for _, c := range user.WebAuthnCredentials() {
		excludeList = append(excludeList, protocol.CredentialDescriptor{
			Type:         protocol.PublicKeyCredentialType,
			CredentialID: c.ID,
			Transport:    c.Transport,
		})
	}

	creation, sessionData, err := d.auth.WebAuthn.WA().BeginRegistration(
		user,
		gowebauthn.WithExclusions(excludeList),
	)
	if err != nil {
		d.Log.Error("begin registration", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "begin failed")
		return
	}
	token, err := d.auth.WebAuthn.PackSession(sessionData)
	if err != nil {
		d.Log.Error("pack session", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "session pack failed")
		return
	}
	optsBytes, err := json.Marshal(creation)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "marshal options")
		return
	}
	writeJSON(w, http.StatusOK, registerBeginResponse{
		Options:      optsBytes,
		SessionToken: token,
		PRFSaltB64:   base64.RawURLEncoding.EncodeToString(op.PrfSalt),
	})
}

type registerFinishBody struct {
	CredentialResponse json.RawMessage `json:"credential_response"`
	SessionToken       string          `json:"session_token"`
	Label              string          `json:"label"`
	WrappedMasterB64   string          `json:"wrapped_master_b64"`
	WrapIvB64          string          `json:"wrap_iv_b64"`
}

// POST /api/admin/auth/register/finish
func (d *Deps) AuthRegisterFinish(w http.ResponseWriter, r *http.Request) {
	if d.auth == nil {
		writeError(w, http.StatusInternalServerError, "internal", "webauthn not configured")
		return
	}
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

	var body registerFinishBody
	if err := readJSON(r, 64*1024, &body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	body.Label = trimAndCap(body.Label, 64)
	if body.Label == "" {
		body.Label = "Security Key"
	}
	var wrappedMaster, wrapIv []byte
	if body.WrappedMasterB64 != "" {
		var err error
		wrappedMaster, err = decodeB64(body.WrappedMasterB64)
		if err != nil || len(wrappedMaster) < 16 || len(wrappedMaster) > 256 {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid wrapped_master_b64")
			return
		}
		wrapIv, err = decodeB64(body.WrapIvB64)
		if err != nil || len(wrapIv) != 12 {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid wrap_iv_b64 (need 12 bytes)")
			return
		}
	}

	sessionData, err := d.auth.WebAuthn.UnpackSession(body.SessionToken)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_session", err.Error())
		return
	}

	creds, err := d.Queries.ListCredentialsByOperator(r.Context(), op.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "list creds")
		return
	}
	user := gogwebauthn.NewOperator(op, creds)

	// Recreate the *http.Request expected by go-webauthn from the JSON body.
	subReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "/", strings.NewReader(string(body.CredentialResponse)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "build subreq")
		return
	}
	subReq.Header.Set("Content-Type", "application/json")

	cred, err := d.auth.WebAuthn.WA().FinishRegistration(user, *sessionData, subReq)
	if err != nil {
		d.Log.Warn("finish registration", "err", err)
		writeError(w, http.StatusBadRequest, "webauthn_failed", err.Error())
		return
	}

	stored, err := d.Queries.CreateCredential(r.Context(), db.CreateCredentialParams{
		OperatorID:    op.ID,
		CredentialID:  cred.ID,
		PublicKey:     cred.PublicKey,
		SignCount:     int64(cred.Authenticator.SignCount),
		Transports:    gogwebauthn.SerializeTransports(cred.Transport),
		Label:         body.Label,
		Aaguid:        cred.Authenticator.AAGUID,
		WrappedMaster: wrappedMaster,
		WrapIv:        wrapIv,
	})
	if err != nil {
		d.Log.Error("store credential", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "store failed")
		return
	}

	d.Audit.Log(r.Context(), audit.Entry{
		Actor: "operator:" + u.Username, Action: "credential.register",
		OperatorID: &op.ID, Request: r,
		Metadata: map[string]any{"credential_id": stored.ID.String(), "label": stored.Label},
	})

	writeJSON(w, http.StatusCreated, credentialSummary(stored))
}

// Login Begin/Finish moved to login.go — those endpoints no longer require an
// existing auth context (they ARE the authentication path). The response
// types loginBeginResponse and loginFinishResponse used by login.go are
// declared below since multiple files reference them.

type loginBeginResponse struct {
	Options      json.RawMessage `json:"options"`
	SessionToken string          `json:"session_token"`
	PRFSaltB64   string          `json:"prf_salt_b64"`
}

type loginFinishResponse struct {
	CredentialIDB64  string `json:"credential_id_b64"`
	WrappedMasterB64 string `json:"wrapped_master_b64"`
	WrapIvB64        string `json:"wrap_iv_b64"`
}

// --- credential management ------------------------------------------------

type credentialJSON struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	Transports []string `json:"transports"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at"`
}

func credentialSummary(c db.WebauthnCredential) credentialJSON {
	out := credentialJSON{
		ID:         c.ID.String(),
		Label:      c.Label,
		Transports: c.Transports,
		CreatedAt:  c.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if c.LastUsedAt.Valid {
		t := c.LastUsedAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		out.LastUsedAt = &t
	}
	return out
}

// GET /api/admin/auth/credentials
func (d *Deps) AuthListCredentials(w http.ResponseWriter, r *http.Request) {
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
	rows, err := d.Queries.ListCredentialsByOperator(r.Context(), op.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "list failed")
		return
	}
	out := make([]credentialJSON, 0, len(rows))
	for _, c := range rows {
		out = append(out, credentialSummary(c))
	}
	writeJSON(w, http.StatusOK, out)
}

// DELETE /api/admin/auth/credentials/{id}
func (d *Deps) AuthDeleteCredential(w http.ResponseWriter, r *http.Request) {
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

	// Refuse to delete the last credential — losing it would lock the operator
	// out and any pending wrapped material would become undecryptable.
	n, err := d.Queries.CountCredentialsByOperator(r.Context(), op.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "count failed")
		return
	}
	if n <= 1 {
		writeError(w, http.StatusConflict, "last_credential",
			"can't delete the last credential; register a replacement first")
		return
	}

	if err := d.Queries.DeleteCredential(r.Context(), db.DeleteCredentialParams{
		ID:         id,
		OperatorID: op.ID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "delete failed")
		return
	}
	d.Audit.Log(r.Context(), audit.Entry{
		Actor: "operator:" + u.Username, Action: "credential.delete",
		OperatorID: &op.ID, Request: r,
		Metadata: map[string]any{"credential_id": id.String()},
	})
	w.WriteHeader(http.StatusNoContent)
}
