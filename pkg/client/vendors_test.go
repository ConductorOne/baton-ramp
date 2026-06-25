package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListVendorsAddsPageSizeToCursorPages(t *testing.T) {
	ctx := context.Background()
	var requests []*http.Request

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Clone(ctx))
		w.Header().Set("Content-Type", "application/json")
		switch len(requests) {
		case 1:
			_, _ = w.Write([]byte(`{"data":[],"page":{"next":"` + server.URL + `/developer/v1/vendors?start=cursor-1"}}`))
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

	firstPage, _, err := c.ListVendors(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if firstPage.Pagination == "" {
		t.Fatal("expected first page to return pagination token")
	}
	if _, _, err := c.ListVendors(ctx, firstPage.Pagination); err != nil {
		t.Fatal(err)
	}

	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requests))
	}
	if requests[0].URL.Query().Get("page_size") != "100" {
		t.Fatalf("first page_size query = %q, want 100", requests[0].URL.Query().Get("page_size"))
	}
	secondQuery := requests[1].URL.Query()
	if secondQuery.Get("page_size") != "100" {
		t.Fatalf("second page_size query = %q, want 100", secondQuery.Get("page_size"))
	}
	if secondQuery.Get("start") != "cursor-1" {
		t.Fatalf("second start query = %q, want cursor-1", secondQuery.Get("start"))
	}
}
