// Package audit writes append-only audit log entries for security-relevant
// actions: who did what, from which IP, with which user-agent, and when.
//
// All writes go through Logger.Log which is fire-and-forget on a background
// goroutine: a failed audit insert must never block or fail the operation
// being audited. The background insert uses a detached context so request
// cancellation doesn't cancel the audit write.
package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/timhasenkamp/gograb/internal/db"
)

// Entry is the input to Logger.Log. Request is optional — when set, IP and
// User-Agent are extracted from it. Metadata is marshalled to JSONB.
type Entry struct {
	Actor      string
	Action     string
	RequestID  *uuid.UUID
	OperatorID *uuid.UUID
	Request    *http.Request
	Metadata   map[string]any
}

// Logger wraps the queries needed to insert audit rows.
type Logger struct {
	queries db.Querier
	log     *slog.Logger
}

func New(q db.Querier, log *slog.Logger) *Logger {
	return &Logger{queries: q, log: log}
}

// Log enqueues an audit entry. Returns immediately; insertion happens
// asynchronously. Failures are logged via slog and do not propagate.
func (l *Logger) Log(_ context.Context, e Entry) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		l.insert(ctx, e)
	}()
}

func (l *Logger) insert(ctx context.Context, e Entry) {
	var ip *netip.Addr
	var ua *string
	if e.Request != nil {
		if addr := extractIP(e.Request); addr.IsValid() {
			ip = &addr
		}
		if v := e.Request.UserAgent(); v != "" {
			s := truncate(v, 256)
			ua = &s
		}
	}
	meta := []byte("{}")
	if len(e.Metadata) > 0 {
		if b, err := json.Marshal(e.Metadata); err == nil {
			meta = b
		}
	}
	if err := l.queries.InsertAuditLog(ctx, db.InsertAuditLogParams{
		Actor:      e.Actor,
		Action:     e.Action,
		RequestID:  e.RequestID,
		OperatorID: e.OperatorID,
		Ip:         ip,
		UserAgent:  ua,
		Metadata:   meta,
	}); err != nil {
		l.log.Warn("audit insert failed", "err", err, "action", e.Action)
	}
}

// extractIP returns the first parsable IP from X-Forwarded-For, falling back
// to RemoteAddr. Returns the zero netip.Addr if none can be parsed.
func extractIP(r *http.Request) netip.Addr {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if comma := strings.Index(v, ","); comma >= 0 {
			v = v[:comma]
		}
		if a, err := netip.ParseAddr(strings.TrimSpace(v)); err == nil {
			return a
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if a, err := netip.ParseAddr(host); err == nil {
		return a
	}
	return netip.Addr{}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
