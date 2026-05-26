package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/timhasenkamp/gograb/internal/auth"
	"github.com/timhasenkamp/gograb/internal/db"
)

type auditEntryJSON struct {
	ID         int64   `json:"id"`
	OccurredAt string  `json:"occurred_at"`
	Actor      string  `json:"actor"`
	Action     string  `json:"action"`
	RequestID  *string `json:"request_id"`
	IP         *string `json:"ip"`
	UserAgent  *string `json:"user_agent"`
}

func toAuditJSON(e db.AuditLog) auditEntryJSON {
	out := auditEntryJSON{
		ID:         e.ID,
		OccurredAt: e.OccurredAt.Time.UTC().Format(time.RFC3339),
		Actor:      e.Actor,
		Action:     e.Action,
		UserAgent:  e.UserAgent,
	}
	if e.RequestID != nil {
		s := e.RequestID.String()
		out.RequestID = &s
	}
	if e.Ip != nil {
		s := e.Ip.String()
		out.IP = &s
	}
	return out
}

// GET /api/admin/audit?limit=100
func (d *Deps) AdminAudit(w http.ResponseWriter, r *http.Request) {
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
	limit := int32(100)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = int32(n)
		}
	}
	rows, err := d.Queries.ListAuditByOperator(r.Context(), db.ListAuditByOperatorParams{
		OperatorID: &op.ID,
		Limit:      limit,
	})
	if err != nil {
		d.Log.Error("list audit", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "list audit failed")
		return
	}
	out := make([]auditEntryJSON, 0, len(rows))
	for _, e := range rows {
		out = append(out, toAuditJSON(e))
	}
	writeJSON(w, http.StatusOK, out)
}
