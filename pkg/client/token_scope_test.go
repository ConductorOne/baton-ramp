package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	testClientSecret      = "test-client-secret"
	testReadScopes        = "users:read vendors:read"
	testUsersWriteScope   = "users:read vendors:read users:write"
	testVendorsWriteScope = "users:read vendors:read vendors:write"
)

type tokenTestServer struct {
	*httptest.Server

	mu            sync.Mutex
	tokenRequests []string
	authByPath    map[string]string
	failScope     string
}

func newTokenTestServer(t *testing.T, failScope string) *tokenTestServer {
	t.Helper()
	ts := &tokenTestServer{authByPath: map[string]string{}, failScope: failScope}
	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/developer/v1/token" {
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse token form: %v", err)
			}
			scope := r.PostForm.Get("scope")
			ts.mu.Lock()
			ts.tokenRequests = append(ts.tokenRequests, scope)
			ts.mu.Unlock()
			if ts.failScope != "" && strings.Contains(scope, ts.failScope) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_scope","secret_echo":"` + testClientSecret + `"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok:" + scope, "token_type": "Bearer", "expires_in": 3600})
			return
		}

		ts.mu.Lock()
		ts.authByPath[r.Method+" "+r.URL.Path] = r.Header.Get("Authorization")
		ts.mu.Unlock()
		switch {
		case r.URL.Path == "/developer/v1/users/deferred":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"task-1"}`))
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"data":[],"page":{"next":""}}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

func (ts *tokenTestServer) tokenSource(ctx context.Context, scopes string) oauth2.TokenSource {
	cfg := &clientcredentials.Config{
		ClientID:     "test-client-id",
		ClientSecret: testClientSecret,
		TokenURL:     TokenURL(ts.URL),
		Scopes:       strings.Fields(scopes),
	}
	return cfg.TokenSource(ctx)
}

func (ts *tokenTestServer) client(t *testing.T, ctx context.Context) *Client {
	t.Helper()
	c, err := NewWithTokenSources(ctx, TokenSources{
		Read:         ts.tokenSource(ctx, testReadScopes),
		UsersWrite:   ts.tokenSource(ctx, testUsersWriteScope),
		VendorsWrite: ts.tokenSource(ctx, testVendorsWriteScope),
		ReadScopes:   strings.Fields(testReadScopes),
	}, ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (ts *tokenTestServer) auth(key string) string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.authByPath[key]
}

func (ts *tokenTestServer) tokenRequestCount(scope string) int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	n := 0
	for _, s := range ts.tokenRequests {
		if s == scope {
			n++
		}
	}
	return n
}

func TestCreateUser_UsesUsersWriteToken(t *testing.T) {
	ctx := context.Background()
	ts := newTokenTestServer(t, "")
	c := ts.client(t, ctx)

	if _, _, err := c.CreateUser(ctx, &CreateUserRequest{}); err != nil {
		t.Fatal(err)
	}
	if got, want := ts.auth("POST /developer/v1/users/deferred"), "Bearer tok:"+testUsersWriteScope; got != want {
		t.Fatalf("Authorization = %q, want %q", got, want)
	}
}

func TestDeactivateReactivate_UseUsersWriteToken(t *testing.T) {
	ctx := context.Background()
	ts := newTokenTestServer(t, "")
	c := ts.client(t, ctx)

	if _, err := c.DeactivateUser(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ReactivateUser(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	want := "Bearer tok:" + testUsersWriteScope
	for _, key := range []string{"PATCH /developer/v1/users/u1/deactivate", "PATCH /developer/v1/users/u1/reactivate"} {
		if got := ts.auth(key); got != want {
			t.Fatalf("%s Authorization = %q, want %q", key, got, want)
		}
	}
}

func TestUpdateVendorOwner_UsesVendorsWriteToken(t *testing.T) {
	ctx := context.Background()
	ts := newTokenTestServer(t, "")
	c := ts.client(t, ctx)

	if _, err := c.UpdateVendorOwner(ctx, "v1", "u1"); err != nil {
		t.Fatal(err)
	}
	if got, want := ts.auth("PATCH /developer/v1/vendors/v1"), "Bearer tok:"+testVendorsWriteScope; got != want {
		t.Fatalf("Authorization = %q, want %q", got, want)
	}
}

func TestReadCalls_NeverFetchWriteToken(t *testing.T) {
	ctx := context.Background()
	ts := newTokenTestServer(t, "")
	c := ts.client(t, ctx)

	if _, _, err := c.ListUsers(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.ListVendors(ctx, ""); err != nil {
		t.Fatal(err)
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if len(ts.tokenRequests) != 1 || ts.tokenRequests[0] != testReadScopes {
		t.Fatalf("token requests = %q, want only %q", ts.tokenRequests, testReadScopes)
	}
}

func TestWriteToken_InvalidScope_FailedPrecondition(t *testing.T) {
	ctx := context.Background()
	ts := newTokenTestServer(t, "users:write")
	c := ts.client(t, ctx)

	_, _, err := c.CreateUser(ctx, &CreateUserRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if code := status.Code(err); code != codes.FailedPrecondition {
		t.Fatalf("status code = %v, want FailedPrecondition (err: %v)", code, err)
	}
	if !strings.Contains(err.Error(), "users:write") {
		t.Fatalf("error should name users:write: %v", err)
	}
	if strings.Contains(err.Error(), testClientSecret) {
		t.Fatalf("error leaks the client secret: %v", err)
	}
	var retrieveErr *oauth2.RetrieveError
	if !errors.As(err, &retrieveErr) {
		t.Fatalf("expected *oauth2.RetrieveError in chain: %v", err)
	}
}

func TestVendorsWriteMissing_UsersWriteStillWorks(t *testing.T) {
	ctx := context.Background()
	ts := newTokenTestServer(t, "vendors:write")
	c := ts.client(t, ctx)

	if _, _, err := c.CreateUser(ctx, &CreateUserRequest{}); err != nil {
		t.Fatalf("CreateUser should succeed without vendors:write: %v", err)
	}
	_, err := c.UpdateVendorOwner(ctx, "v1", "u1")
	if code := status.Code(err); code != codes.FailedPrecondition {
		t.Fatalf("status code = %v, want FailedPrecondition (err: %v)", code, err)
	}
	if !strings.Contains(err.Error(), "vendors:write") {
		t.Fatalf("error should name vendors:write: %v", err)
	}
}

func TestReadToken_InvalidScope_FailedPrecondition(t *testing.T) {
	ctx := context.Background()
	ts := newTokenTestServer(t, "users:read")
	c := ts.client(t, ctx)

	_, _, err := c.ListUsers(ctx, "")
	if code := status.Code(err); code != codes.FailedPrecondition {
		t.Fatalf("status code = %v, want FailedPrecondition (err: %v)", code, err)
	}
	if !strings.Contains(err.Error(), testReadScopes) {
		t.Fatalf("error should list the read scopes: %v", err)
	}
}

func TestStaticToken_AllClasses(t *testing.T) {
	ctx := context.Background()
	ts := newTokenTestServer(t, "")
	c, err := New(ctx, Token{AccessToken: "static"}, ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := c.ListUsers(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.CreateUser(ctx, &CreateUserRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UpdateVendorOwner(ctx, "v1", "u1"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"GET /developer/v1/users", "POST /developer/v1/users/deferred", "PATCH /developer/v1/vendors/v1"} {
		if got := ts.auth(key); got != "Bearer static" {
			t.Fatalf("%s Authorization = %q, want %q", key, got, "Bearer static")
		}
	}
}

func TestTokenSourcesCached(t *testing.T) {
	ctx := context.Background()
	ts := newTokenTestServer(t, "")
	c := ts.client(t, ctx)

	for range 2 {
		if _, _, err := c.CreateUser(ctx, &CreateUserRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	if n := ts.tokenRequestCount(testUsersWriteScope); n != 1 {
		t.Fatalf("users:write token requests = %d, want 1", n)
	}
}
