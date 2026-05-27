package audit

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/timhasenkamp/gograb/internal/db"
)

// fakeQ embeds db.Querier so only the method we exercise needs to be set.
// Calls to any other method will nil-deref panic — that's the desired
// "loud failure" if a test accidentally hits a path we didn't expect.
type fakeQ struct {
	db.Querier
	inserts atomic.Int32
	last    db.InsertAuditLogParams
	err     error
}

func (f *fakeQ) InsertAuditLog(_ context.Context, arg db.InsertAuditLogParams) error {
	f.inserts.Add(1)
	f.last = arg
	return f.err
}

func newLogger(buf *bytes.Buffer) *slog.Logger {
	if buf == nil {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func TestLogger_InsertHappyPath(t *testing.T) {
	fq := &fakeQ{}
	l := New(fq, newLogger(nil))

	req := httptest.NewRequest("POST", "/test", nil)
	req.RemoteAddr = "10.0.0.5:55555"
	req.Header.Set("User-Agent", "test-agent/1.0")
	rid := uuid.New()
	opid := uuid.New()

	// Call the unexported synchronous insert directly so we don't have to race
	// with the background goroutine that Log() spawns.
	l.insert(context.Background(), Entry{
		Actor: "operator:alice", Action: "request.create",
		RequestID: &rid, OperatorID: &opid, Request: req,
		Metadata: map[string]any{"description": "x"},
	})

	if got := fq.inserts.Load(); got != 1 {
		t.Fatalf("inserts = %d, want 1", got)
	}
	if fq.last.Actor != "operator:alice" || fq.last.Action != "request.create" {
		t.Errorf("actor/action mismatch: %+v", fq.last)
	}
	if fq.last.Ip == nil || fq.last.Ip.String() != "10.0.0.5" {
		t.Errorf("ip mismatch: %v", fq.last.Ip)
	}
	if fq.last.UserAgent == nil || *fq.last.UserAgent != "test-agent/1.0" {
		t.Errorf("user-agent mismatch: %v", fq.last.UserAgent)
	}
}

func TestLogger_InsertSwallowsDBError(t *testing.T) {
	buf := &bytes.Buffer{}
	fq := &fakeQ{err: errors.New("boom")}
	l := New(fq, newLogger(buf))

	// insert should not panic and should log a warning.
	l.insert(context.Background(), Entry{Actor: "x", Action: "y"})

	if fq.inserts.Load() != 1 {
		t.Errorf("expected insert call even on error path")
	}
	if !bytes.Contains(buf.Bytes(), []byte("audit insert failed")) {
		t.Errorf("expected warning log, got: %s", buf.String())
	}
}

func TestLogger_InsertHandlesXForwardedFor(t *testing.T) {
	fq := &fakeQ{}
	l := New(fq, newLogger(nil))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:1" // Traefik
	req.Header.Set("X-Forwarded-For", "203.0.113.42, 10.0.0.1")

	l.insert(context.Background(), Entry{Actor: "customer", Action: "request.submit", Request: req})

	if fq.last.Ip == nil || fq.last.Ip.String() != "203.0.113.42" {
		t.Errorf("expected first XFF entry, got %v", fq.last.Ip)
	}
}

func TestLogger_TruncatesLongUserAgent(t *testing.T) {
	fq := &fakeQ{}
	l := New(fq, newLogger(nil))
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'A'
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("User-Agent", string(long))

	l.insert(context.Background(), Entry{Actor: "x", Action: "y", Request: req})

	if fq.last.UserAgent == nil || len(*fq.last.UserAgent) != 256 {
		t.Errorf("expected UA truncated to 256, got %d", len(*fq.last.UserAgent))
	}
}
