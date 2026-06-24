package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListVendorAgreementsUsesPostBodyForCursorPages(t *testing.T) {
	ctx := context.Background()
	var requests []struct {
		method string
		path   string
		body   string
	}

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		requests = append(requests, struct {
			method string
			path   string
			body   string
		}{
			method: r.Method,
			path:   r.URL.RequestURI(),
			body:   string(body),
		})

		w.Header().Set("Content-Type", "application/json")
		if len(requests) == 1 {
			_, _ = w.Write([]byte(`{"data":[],"page":{"next":"` + server.URL + `/developer/v1/vendors/agreements?start=cursor-1"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[],"page":{"next":""}}`))
	}))
	defer server.Close()

	c, err := New(ctx, Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	req := &VendorAgreementsListRequest{PageSize: 25}
	firstPage, _, err := c.ListVendorAgreements(ctx, req, "")
	if err != nil {
		t.Fatal(err)
	}
	if firstPage.Pagination == "" {
		t.Fatal("expected first page to return pagination token")
	}

	if _, _, err := c.ListVendorAgreements(ctx, req, firstPage.Pagination); err != nil {
		t.Fatal(err)
	}

	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requests))
	}
	for i, request := range requests {
		if request.method != http.MethodPost {
			t.Fatalf("request %d method = %s, want POST", i, request.method)
		}
		if !strings.Contains(request.body, `"page_size":25`) {
			t.Fatalf("request %d missing search body: %s", i, request.body)
		}
	}
	if requests[0].path != "/developer/v1/vendors/agreements" {
		t.Fatalf("first path = %s", requests[0].path)
	}
	if requests[1].path != "/developer/v1/vendors/agreements?start=cursor-1" {
		t.Fatalf("cursor path = %s", requests[1].path)
	}
}

func TestVendorAgreementsListUnmarshalBareArray(t *testing.T) {
	var got VendorAgreementsList
	if err := json.Unmarshal([]byte(`[{"id":"agreement-1","name":"MSA"}]`), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Data) != 1 {
		t.Fatalf("agreements = %d, want 1", len(got.Data))
	}
	if got.Data[0].ID != "agreement-1" {
		t.Fatalf("agreement id = %q, want agreement-1", got.Data[0].ID)
	}
	if got.Page.Next != "" {
		t.Fatalf("page next = %q, want empty", got.Page.Next)
	}
}
