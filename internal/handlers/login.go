package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/timhasenkamp/gograb/internal/audit"
	"github.com/timhasenkamp/gograb/internal/auth"
	"github.com/timhasenkamp/gograb/internal/db"
	"github.com/timhasenkamp/gograb/internal/session"
	gogwebauthn "github.com/timhasenkamp/gograb/internal/webauthn"
)

// SessionDeps bundles the things login/signup endpoints need beyond the
// regular Deps. Constructed once in main and attached via WithSession.
type SessionDeps struct {
	Manager *session.Manager
	// AllowSignup gates the /api/admin/auth/signup/* endpoints. Set to false
	// (via GOGRAB_SIGNUP_ENABLED=false) to lock down a finished install.
	AllowSignup bool
}

// WithSession attaches login deps. Returns d for chaining.
func (d *Deps) WithSession(s *SessionDeps) *Deps {
	d.sessionDeps = s
	return d
}

// usernameRE bounds usernames to safe, URL-friendly characters. Length is
// checked separately so we can give a clearer error message.
var usernameRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]{1,30}[a-z0-9])?$`)

// normalizeUsername lower-cases and trims. The on-disk username always has
// this canonical form so case-different signups don't collide silently.
func normalizeUsername(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// validateEmail is permissive — we don't bounce mail through it, it's just
// a display field. Reject obviously malformed input but no MX checks etc.
func validateEmail(s string) bool {
	if s == "" {
		return true
	}
	if len(s) > 254 {
		return false
	}
	at := strings.IndexByte(s, '@')
	return at > 0 && at < len(s)-1 && !strings.ContainsAny(s, " \t\n\r")
}

// --- signup ----------------------------------------------------------------

type signupBeginBody struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

// POST /api/admin/auth/signup/begin
func (d *Deps) AuthSignupBegin(w http.ResponseWriter, r *http.Request) {
	if d.auth == nil || d.sessionDeps == nil {
		writeError(w, http.StatusInternalServerError, "internal", "auth not configured")
		return
	}
	if !d.sessionDeps.AllowSignup {
		writeError(w, http.StatusForbidden, "signup_disabled", "self-signup is disabled on this instance")
		return
	}

	var body signupBeginBody
	if err := readJSON(r, 4096, &body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	body.Username = normalizeUsername(body.Username)
	body.Email = strings.TrimSpace(body.Email)
	if !usernameRE.MatchString(body.Username) {
		writeError(w, http.StatusBadRequest, "bad_username",
			"username must be 2-32 chars, lowercase letters/digits/._- only")
		return
	}
	if !validateEmail(body.Email) {
		writeError(w, http.StatusBadRequest, "bad_email", "invalid email")
		return
	}

	op, err := d.Queries.GetOperatorByUsername(r.Context(), body.Username)
	switch {
	case err == nil:
		// Existing operator: only resumable if no credentials are bound yet.
		n, cerr := d.Queries.CountCredentialsByOperator(r.Context(), op.ID)
		if cerr != nil {
			d.Log.Error("count creds", "err", cerr)
			writeError(w, http.StatusInternalServerError, "internal", "lookup failed")
			return
		}
		if n > 0 {
			writeError(w, http.StatusConflict, "username_taken", "username already exists")
			return
		}
		// Fall through with the existing op row; PRF salt is reused.
	case errors.Is(err, pgx.ErrNoRows):
		salt := make([]byte, 32)
		if _, gerr := rand.Read(salt); gerr != nil {
			writeError(w, http.StatusInternalServerError, "internal", "salt gen failed")
			return
		}
		op, err = d.Queries.CreateOperator(r.Context(), db.CreateOperatorParams{
			Username: body.Username,
			Email:    body.Email,
			PrfSalt:  salt,
		})
		if err != nil {
			d.Log.Error("create operator", "err", err)
			writeError(w, http.StatusInternalServerError, "internal", "create operator failed")
			return
		}
	default:
		d.Log.Error("get operator", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "lookup failed")
		return
	}

	user := gogwebauthn.NewOperator(op, nil) // no creds yet during signup
	creation, sessionData, err := d.auth.WebAuthn.WA().BeginRegistration(user)
	if err != nil {
		d.Log.Error("begin registration", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "begin failed")
		return
	}
	tok, err := d.auth.WebAuthn.PackSession(sessionData)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "session pack failed")
		return
	}
	optsBytes, err := json.Marshal(creation)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "marshal options")
		return
	}

	d.Audit.Log(r.Context(), audit.Entry{
		Actor: "anonymous", Action: "signup.begin",
		OperatorID: &op.ID, Request: r,
		Metadata: map[string]any{"username": op.Username},
	})

	writeJSON(w, http.StatusOK, registerBeginResponse{
		Options:      optsBytes,
		SessionToken: tok,
		PRFSaltB64:   base64.RawURLEncoding.EncodeToString(op.PrfSalt),
	})
}

type signupFinishBody struct {
	Username           string          `json:"username"`
	CredentialResponse json.RawMessage `json:"credential_response"`
	SessionToken       string          `json:"session_token"`
	Label              string          `json:"label"`
	WrappedMasterB64   string          `json:"wrapped_master_b64"`
	WrapIvB64          string          `json:"wrap_iv_b64"`
}

// POST /api/admin/auth/signup/finish
func (d *Deps) AuthSignupFinish(w http.ResponseWriter, r *http.Request) {
	if d.auth == nil || d.sessionDeps == nil {
		writeError(w, http.StatusInternalServerError, "internal", "auth not configured")
		return
	}
	if !d.sessionDeps.AllowSignup {
		writeError(w, http.StatusForbidden, "signup_disabled", "self-signup is disabled on this instance")
		return
	}

	var body signupFinishBody
	if err := readJSON(r, 64*1024, &body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	body.Username = normalizeUsername(body.Username)
	body.Label = trimAndCap(body.Label, 64)
	if body.Label == "" {
		body.Label = "Primary"
	}
	if !usernameRE.MatchString(body.Username) {
		writeError(w, http.StatusBadRequest, "bad_username", "invalid username")
		return
	}
	// wrapped_master is optional: some authenticator+browser combinations on Linux
	// only return PRF during get() (assertion), not create() (registration).
	// If omitted, the client must call /signup/set-master after a second assertion.
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
			writeError(w, http.StatusBadRequest, "bad_request", "invalid wrap_iv_b64")
			return
		}
	}

	op, err := d.Queries.GetOperatorByUsername(r.Context(), body.Username)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unknown_user", "no signup in progress for this user")
		return
	}
	n, err := d.Queries.CountCredentialsByOperator(r.Context(), op.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "lookup failed")
		return
	}
	if n > 0 {
		writeError(w, http.StatusConflict, "already_registered", "this account is already set up; use login instead")
		return
	}

	sessionData, err := d.auth.WebAuthn.UnpackSession(body.SessionToken)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_session", err.Error())
		return
	}

	user := gogwebauthn.NewOperator(op, nil)
	subReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "/", strings.NewReader(string(body.CredentialResponse)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "build subreq")
		return
	}
	subReq.Header.Set("Content-Type", "application/json")

	cred, err := d.auth.WebAuthn.WA().FinishRegistration(user, *sessionData, subReq)
	if err != nil {
		d.Log.Warn("finish signup", "err", err)
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

	d.sessionDeps.Manager.SetCookie(w, d.sessionDeps.Manager.Issue(op.ID))

	d.Audit.Log(r.Context(), audit.Entry{
		Actor: "operator:" + op.Username, Action: "signup.finish",
		OperatorID: &op.ID, Request: r,
		Metadata: map[string]any{"credential_id": stored.ID.String(), "label": stored.Label},
	})

	writeJSON(w, http.StatusCreated, credentialSummary(stored))
}

// --- login -----------------------------------------------------------------

type loginBeginBody struct {
	Username string `json:"username"`
}

// POST /api/admin/auth/login/begin
//
// Unauthenticated. Client sends a username; we return WebAuthn assertion
// options listing only the credentials registered under that account.
func (d *Deps) AuthLoginBegin(w http.ResponseWriter, r *http.Request) {
	if d.auth == nil || d.sessionDeps == nil {
		writeError(w, http.StatusInternalServerError, "internal", "auth not configured")
		return
	}
	var body loginBeginBody
	if err := readJSON(r, 4096, &body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	body.Username = normalizeUsername(body.Username)
	if !usernameRE.MatchString(body.Username) {
		writeError(w, http.StatusBadRequest, "bad_username", "invalid username")
		return
	}
	op, err := d.Queries.GetOperatorByUsername(r.Context(), body.Username)
	if err != nil {
		// Generic message — same code path whether the user exists or not.
		writeError(w, http.StatusUnauthorized, "unknown_user", "no such account")
		return
	}
	creds, err := d.Queries.ListCredentialsByOperator(r.Context(), op.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "list failed")
		return
	}
	if len(creds) == 0 {
		writeError(w, http.StatusConflict, "no_credentials", "this account has no registered authenticator")
		return
	}
	user := gogwebauthn.NewOperator(op, creds)
	assertion, sessionData, err := d.auth.WebAuthn.WA().BeginLogin(user)
	if err != nil {
		d.Log.Error("begin login", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "begin failed")
		return
	}
	tok, err := d.auth.WebAuthn.PackSession(sessionData)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "session pack failed")
		return
	}
	optsBytes, err := json.Marshal(assertion)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "marshal options")
		return
	}
	writeJSON(w, http.StatusOK, loginBeginResponse{
		Options:      optsBytes,
		SessionToken: tok,
		PRFSaltB64:   base64.RawURLEncoding.EncodeToString(op.PrfSalt),
	})
}

type loginFinishBodyWithUser struct {
	Username           string          `json:"username"`
	CredentialResponse json.RawMessage `json:"credential_response"`
	SessionToken       string          `json:"session_token"`
}

// POST /api/admin/auth/login/finish
//
// Verifies the WebAuthn assertion, sets a session cookie, and returns the
// wrapped Master-KEK material for the credential that was used.
func (d *Deps) AuthLoginFinish(w http.ResponseWriter, r *http.Request) {
	if d.auth == nil || d.sessionDeps == nil {
		writeError(w, http.StatusInternalServerError, "internal", "auth not configured")
		return
	}
	var body loginFinishBodyWithUser
	if err := readJSON(r, 64*1024, &body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	body.Username = normalizeUsername(body.Username)
	if !usernameRE.MatchString(body.Username) {
		writeError(w, http.StatusBadRequest, "bad_username", "invalid username")
		return
	}

	op, err := d.Queries.GetOperatorByUsername(r.Context(), body.Username)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unknown_user", "no such account")
		return
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
	subReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "/", strings.NewReader(string(body.CredentialResponse)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "build subreq")
		return
	}
	subReq.Header.Set("Content-Type", "application/json")

	usedCred, err := d.auth.WebAuthn.WA().FinishLogin(user, *sessionData, subReq)
	if err != nil {
		d.Log.Warn("finish login", "err", err)
		writeError(w, http.StatusUnauthorized, "webauthn_failed", err.Error())
		return
	}
	stored, err := d.Queries.GetCredentialByCredentialID(r.Context(), usedCred.ID)
	if err != nil || stored.OperatorID != op.ID {
		writeError(w, http.StatusUnauthorized, "unknown_credential", "credential not found")
		return
	}
	if err := d.Queries.UpdateCredentialAfterUse(r.Context(), db.UpdateCredentialAfterUseParams{
		ID:        stored.ID,
		SignCount: int64(usedCred.Authenticator.SignCount),
	}); err != nil {
		d.Log.Warn("update sign_count", "err", err)
	}

	d.sessionDeps.Manager.SetCookie(w, d.sessionDeps.Manager.Issue(op.ID))

	d.Audit.Log(r.Context(), audit.Entry{
		Actor: "operator:" + op.Username, Action: "session.login",
		OperatorID: &op.ID, Request: r,
		Metadata: map[string]any{"credential_id": stored.ID.String(), "label": stored.Label},
	})

	writeJSON(w, http.StatusOK, loginFinishResponse{
		CredentialIDB64:  base64.RawURLEncoding.EncodeToString(stored.CredentialID),
		WrappedMasterB64: base64.RawURLEncoding.EncodeToString(stored.WrappedMaster),
		WrapIvB64:        base64.RawURLEncoding.EncodeToString(stored.WrapIv),
	})
}

// --- signup set-master (two-shot PRF fallback) --------------------------------

type signupSetMasterBody struct {
	CredentialID     string `json:"credential_id"`
	WrappedMasterB64 string `json:"wrapped_master_b64"`
	WrapIvB64        string `json:"wrap_iv_b64"`
}

// POST /api/admin/auth/signup/set-master
//
// Second step of the two-shot signup path: the client already registered the
// credential via /signup/finish (no master key), then obtained PRF via an
// assertion and sends the wrapped master here. Requires a session cookie that
// was issued by /signup/finish.
func (d *Deps) AuthSignupSetMaster(w http.ResponseWriter, r *http.Request) {
	if d.auth == nil || d.sessionDeps == nil {
		writeError(w, http.StatusInternalServerError, "internal", "auth not configured")
		return
	}
	u, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no user")
		return
	}
	op, err := d.Queries.GetOperatorByUsername(r.Context(), u.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "operator lookup failed")
		return
	}

	var body signupSetMasterBody
	if err := readJSON(r, 4096, &body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	credID, err := uuid.Parse(body.CredentialID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid credential_id")
		return
	}
	wrappedMaster, err := decodeB64(body.WrappedMasterB64)
	if err != nil || len(wrappedMaster) < 16 || len(wrappedMaster) > 256 {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid wrapped_master_b64")
		return
	}
	wrapIv, err := decodeB64(body.WrapIvB64)
	if err != nil || len(wrapIv) != 12 {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid wrap_iv_b64 (need 12 bytes)")
		return
	}

	if _, err := d.Queries.SetCredentialWrappedMaster(r.Context(), db.SetCredentialWrappedMasterParams{
		ID:            credID,
		OperatorID:    op.ID,
		WrappedMaster: wrappedMaster,
		WrapIv:        wrapIv,
	}); err != nil {
		d.Log.Error("set credential wrapped master", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "store failed")
		return
	}

	d.Audit.Log(r.Context(), audit.Entry{
		Actor: "operator:" + u.Username, Action: "credential.set_master",
		OperatorID: &op.ID, Request: r,
		Metadata: map[string]any{"credential_id": credID.String()},
	})

	w.WriteHeader(http.StatusNoContent)
}

// POST /api/admin/auth/logout
func (d *Deps) AuthLogout(w http.ResponseWriter, r *http.Request) {
	if d.sessionDeps == nil {
		writeError(w, http.StatusInternalServerError, "internal", "session not configured")
		return
	}
	if u, ok := auth.FromContext(r.Context()); ok {
		d.Audit.Log(r.Context(), audit.Entry{
			Actor: "operator:" + u.Username, Action: "session.logout",
			Request: r,
		})
	}
	d.sessionDeps.Manager.ClearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// --- session middleware ----------------------------------------------------

// SessionMiddleware turns a signed session cookie into an auth.User on the
// request context. It pairs with d.WithSession; if no manager was attached
// the middleware fails closed.
//
// When required is true, missing or invalid sessions get 401 immediately.
// When false (signup/login endpoints), the request is passed through with
// no auth.User in context.
func (d *Deps) SessionMiddleware(required bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if d.sessionDeps == nil {
				writeError(w, http.StatusInternalServerError, "internal", "session not configured")
				return
			}
			s, err := d.sessionDeps.Manager.FromRequest(r)
			if err != nil {
				if required {
					// Stale cookie? Make sure the browser drops it.
					if !errors.Is(err, http.ErrNoCookie) {
						d.sessionDeps.Manager.ClearCookie(w)
					}
					writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			op, err := d.Queries.GetOperatorByID(r.Context(), s.OperatorID)
			if err != nil {
				// Operator deleted but cookie still around. Clear and reject.
				d.sessionDeps.Manager.ClearCookie(w)
				if required {
					writeError(w, http.StatusUnauthorized, "unauthorized", "session no longer valid")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			ctx := session.WithSession(r.Context(), s)
			ctx = auth.WithUser(ctx, auth.User{Username: op.Username, Email: op.Email})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// DevSessionMiddleware short-circuits authentication for local development.
// On every request it ensures an operator row exists for the configured dev
// user, then injects an auth.User + Session into the context. Used only when
// GOGRAB_DEV_USER is set; never in production builds.
func (d *Deps) DevSessionMiddleware(username, email string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			op, err := d.getOrCreateOperator(r.Context(), auth.User{Username: username, Email: email})
			if err != nil {
				d.Log.Error("dev operator lookup", "err", err)
				writeError(w, http.StatusInternalServerError, "internal", "dev user setup failed")
				return
			}
			ctx := session.WithSession(r.Context(), session.Session{OperatorID: op.ID})
			ctx = auth.WithUser(ctx, auth.User{Username: op.Username, Email: op.Email})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

