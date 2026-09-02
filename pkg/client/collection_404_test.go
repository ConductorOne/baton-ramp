package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A 404 from a collection endpoint must not surface as codes.NotFound: the SDK
// treats NotFound as recoverable (IsSyncPreservable), which is what let a
// truncated user list be preserved and ingested as a completed sync.
func TestListUsers404IsNotPreservableCode(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		body string
		ct   string
	}{
		{name: "json error body", body: `{"error":{"message":"not found"}}`, ct: "application/json"},
		{name: "empty body", body: "", ct: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.ct != "" {
					w.Header().Set("Content-Type", tc.ct)
				}
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			c, err := New(ctx, Token{AccessToken: "token"}, server.URL)
			if err != nil {
				t.Fatal(err)
			}

			resp, _, err := c.ListUsers(ctx, "")
			if err == nil {
				t.Fatalf("expected an error, got resp=%#v", resp)
			}
			if got := status.Code(err); got == codes.NotFound {
				t.Errorf("status code = %v, want anything but NotFound (it is preservable): %v", got, err)
			}
			if got := status.Code(err); got != codes.Internal {
				t.Errorf("status code = %v, want Internal: %v", got, err)
			}
		})
	}
}

// The same guarantee has to hold on a cursor page, which is where a
// mid-pagination 404 truncates the sync.
func TestListUsers404OnCursorPageIsNotPreservableCode(t *testing.T) {
	ctx := context.Background()
	var n atomic.Int32

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if n.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"data":[{"id":"u1","email":"a@b.com","first_name":"A","last_name":"B","status":"USER_ACTIVE","role":"BUSINESS_USER"}],` +
				`"page":{"next":"` + server.URL + `/developer/v1/users?start=cursor-1"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"not found"}}`))
	}))
	defer server.Close()

	c, err := New(ctx, Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	first, _, err := c.ListUsers(ctx, "")
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if first.Pagination == "" {
		t.Fatal("expected a next cursor on the first page")
	}

	if _, _, err = c.ListUsers(ctx, first.Pagination); err == nil {
		t.Fatal("expected an error on the cursor page")
	}
	if got := status.Code(err); got != codes.Internal {
		t.Errorf("status code = %v, want Internal: %v", got, err)
	}
}

// A 404 on a single-resource GET keeps its NotFound classification: there the
// resource genuinely may be gone, and callers rely on telling that apart.
func TestGetVendor404StaysNotFound(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"no such vendor"}}`))
	}))
	defer server.Close()

	c, err := New(ctx, Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err = c.GetVendor(ctx, "v-does-not-exist"); err == nil {
		t.Fatal("expected an error")
	}
	if got := status.Code(err); got != codes.NotFound {
		t.Errorf("status code = %v, want NotFound: %v", got, err)
	}
}

func TestEffectivePageSize(t *testing.T) {
	for _, tc := range []struct {
		url  string
		want int
	}{
		{url: "https://api.ramp.com/developer/v1/users?page_size=100", want: 100},
		{url: "https://api.ramp.com/developer/v1/users?page_size=25&start=abc", want: 25},
		{url: "https://api.ramp.com/developer/v1/users", want: 0},
		{url: "https://api.ramp.com/developer/v1/users?page_size=nope", want: 0},
	} {
		if got := effectivePageSize(tc.url); got != tc.want {
			t.Errorf("effectivePageSize(%q) = %d, want %d", tc.url, got, tc.want)
		}
	}
}

// Re-coding must set the code without flattening the chain, so errors.As can
// still reach the uhttp cause and the original 404 detail survives for logs.
func TestListUsers404PreservesErrorChain(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"not found"}}`))
	}))
	defer server.Close()

	c, err := New(ctx, Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = c.ListUsers(ctx, "")
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := status.Code(err); got != codes.Internal {
		t.Fatalf("status code = %v, want Internal: %v", got, err)
	}

	// The joined cause is still walkable rather than collapsed into a string.
	var unwrapped interface{ GRPCStatus() *status.Status }
	if !errors.As(err, &unwrapped) {
		t.Error("expected a gRPC status to be reachable via errors.As")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected the original 404 detail to survive, got: %v", err)
	}
	if !strings.Contains(err.Error(), "never a benign empty result") {
		t.Errorf("expected the re-coding message, got: %v", err)
	}
}

// The audit-log endpoint is deliberately excluded from re-coding: it feeds the
// event feed, where no partial c1z is at stake, and a 404 there is a plausible
// tenant-configuration signal rather than a connector fault.
func TestListAuditLogEvents404StaysNotFound(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"audit logs not enabled"}}`))
	}))
	defer server.Close()

	c, err := New(ctx, Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, _, err = c.ListAuditLogEvents(ctx, &AuditLogEventsRequest{}, ""); err == nil {
		t.Fatal("expected an error")
	}
	if got := status.Code(err); got != codes.NotFound {
		t.Errorf("status code = %v, want NotFound: %v", got, err)
	}
}

// Vendor agreements are on the sync path, so they do get re-coded.
func TestListVendorAgreements404IsNotPreservableCode(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"not found"}}`))
	}))
	defer server.Close()

	c, err := New(ctx, Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err = c.ListVendorAgreements(ctx, &VendorAgreementsListRequest{}, ""); err == nil {
		t.Fatal("expected an error")
	}
	if got := status.Code(err); got != codes.Internal {
		t.Errorf("status code = %v, want Internal: %v", got, err)
	}
}

func TestListVendors404IsNotPreservableCode(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"not found"}}`))
	}))
	defer server.Close()

	c, err := New(ctx, Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err = c.ListVendors(ctx, ""); err == nil {
		t.Fatal("expected an error")
	}
	if got := status.Code(err); got != codes.Internal {
		t.Errorf("status code = %v, want Internal: %v", got, err)
	}
}
