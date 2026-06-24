package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListUsersByRoleAddsRoleAndPageSizeToCursorPages(t *testing.T) {
	ctx := context.Background()
	var requests []*http.Request

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Clone(ctx))
		w.Header().Set("Content-Type", "application/json")
		switch len(requests) {
		case 1:
			_, _ = w.Write([]byte(`{"data":[],"page":{"next":"` + server.URL + `/developer/v1/users?start=cursor-1"}}`))
		case 2:
			_, _ = w.Write([]byte(`{"data":[],"page":{"next":""}}`))
		default:
			t.Fatalf("unexpected request %d: %s", len(requests), r.URL.RequestURI())
		}
	}))
	defer server.Close()

	c, err := New(ctx, Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	firstPage, _, err := c.ListUsersByRole(ctx, "BUSINESS_USER", "")
	if err != nil {
		t.Fatal(err)
	}
	if firstPage.Pagination == "" {
		t.Fatal("expected first page to return pagination token")
	}
	if _, _, err := c.ListUsersByRole(ctx, "BUSINESS_USER", firstPage.Pagination); err != nil {
		t.Fatal(err)
	}

	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requests))
	}
	firstQuery := requests[0].URL.Query()
	if firstQuery.Get("role") != "BUSINESS_USER" {
		t.Fatalf("first role query = %q, want BUSINESS_USER", firstQuery.Get("role"))
	}
	if firstQuery.Get("page_size") != "100" {
		t.Fatalf("first page_size query = %q, want 100", firstQuery.Get("page_size"))
	}
	if firstQuery.Get("start") != "" {
		t.Fatalf("first start query = %q, want empty", firstQuery.Get("start"))
	}

	secondQuery := requests[1].URL.Query()
	if secondQuery.Get("role") != "BUSINESS_USER" {
		t.Fatalf("second role query = %q, want BUSINESS_USER", secondQuery.Get("role"))
	}
	if secondQuery.Get("page_size") != "100" {
		t.Fatalf("second page_size query = %q, want 100", secondQuery.Get("page_size"))
	}
	if secondQuery.Get("start") != "cursor-1" {
		t.Fatalf("second start query = %q, want cursor-1", secondQuery.Get("start"))
	}
}

func TestListUsersAddsPageSizeWithoutRole(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		query := r.URL.Query()
		if query.Get("role") != "" {
			t.Fatalf("role query = %q, want empty", query.Get("role"))
		}
		if query.Get("page_size") != "100" {
			t.Fatalf("page_size query = %q, want 100", query.Get("page_size"))
		}
		_, _ = w.Write([]byte(`{"data":[],"page":{"next":""}}`))
	}))
	defer server.Close()

	c, err := New(ctx, Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.ListUsers(ctx, ""); err != nil {
		t.Fatal(err)
	}
}
