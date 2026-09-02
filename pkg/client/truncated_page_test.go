package client

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// syncBuffer guards the buffer the test logger writes into; zap cores may be
// written from any goroutine and the race detector runs in CI.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) Sync() error { return nil }

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func ctxWithWarnLogger(t *testing.T) (context.Context, *syncBuffer) {
	t.Helper()
	sb := &syncBuffer{}
	logger := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(sb),
		zapcore.WarnLevel,
	))
	return ctxzap.ToContext(context.Background(), logger), sb
}

// usersPage renders n users plus the given next cursor.
func usersPage(n int, next string) string {
	users := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id := strconv.Itoa(i)
		users = append(users, `{"id":"u`+id+`","email":"a`+id+
			`@b.com","first_name":"A","last_name":"B","status":"USER_ACTIVE","role":"BUSINESS_USER"}`)
	}
	return `{"data":[` + strings.Join(users, ",") + `],"page":{"next":"` + next + `"}}`
}

// The observed failure: a full page comes back with no next cursor, so the
// connector stops with no error and the sync completes holding only part of the
// collection. The count cannot be validated from one response, so the contract
// is that it is at least reported.
func TestListUsersWarnsOnFullPageWithoutCursor(t *testing.T) {
	ctx, logs := ctxWithWarnLogger(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(usersPage(100, "")))
	}))
	defer server.Close()

	c, err := New(ctx, Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	resp, _, err := c.ListUsers(ctx, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Users) != 100 {
		t.Fatalf("got %d users, want 100", len(resp.Users))
	}
	if resp.Pagination != "" {
		t.Fatalf("pagination = %q, want empty", resp.Pagination)
	}

	out := logs.String()
	if !strings.Contains(out, "the list may be truncated") {
		t.Errorf("expected a truncation warning, got: %s", out)
	}
	if !strings.Contains(out, `"page_size":100`) || !strings.Contains(out, `"returned":100`) {
		t.Errorf("warning missing counts, got: %s", out)
	}
}

// A short final page is the normal end of pagination and must stay silent.
func TestListUsersNoWarnOnShortPage(t *testing.T) {
	ctx, logs := ctxWithWarnLogger(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(usersPage(58, "")))
	}))
	defer server.Close()

	c, err := New(ctx, Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = c.ListUsers(ctx, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out := logs.String(); strings.Contains(out, "truncated") {
		t.Errorf("unexpected truncation warning: %s", out)
	}
}

// A full page that does carry a cursor is ordinary mid-pagination traffic.
func TestListUsersNoWarnOnFullPageWithCursor(t *testing.T) {
	ctx, logs := ctxWithWarnLogger(t)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(usersPage(100, server.URL+"/developer/v1/users?start=cursor-1")))
	}))
	defer server.Close()

	c, err := New(ctx, Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = c.ListUsers(ctx, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out := logs.String(); strings.Contains(out, "truncated") {
		t.Errorf("unexpected truncation warning: %s", out)
	}
}
