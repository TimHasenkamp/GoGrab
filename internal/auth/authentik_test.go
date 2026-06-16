package auth

import (
	"context"
	"testing"
)

func TestContextRoundtrip(t *testing.T) {
	ctx := WithUser(context.Background(), User{Username: "alice", Email: "alice@example.com"})
	u, ok := FromContext(ctx)
	if !ok {
		t.Fatal("FromContext returned ok=false on a populated ctx")
	}
	if u.Username != "alice" || u.Email != "alice@example.com" {
		t.Errorf("user = %+v, want {alice alice@example.com}", u)
	}
}

func TestFromContextEmpty(t *testing.T) {
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("empty context unexpectedly produced a User")
	}
}
