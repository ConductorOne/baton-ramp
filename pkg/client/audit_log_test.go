package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFlattenAuditEvents_FlatArray covers compatibility with a flat data
// array. Values are anonymized synthetic fixtures, not live audit data.
func TestFlattenAuditEvents_FlatArray(t *testing.T) {
	body := []byte(`[
		{"id": "ev1", "event_type": "Vendor management agreement status changed", "primary_reference": {"id": "vendor-1", "resource_name": "Vendor / Merchant", "url": "/vendors/vendor-1"}},
		{"id": "ev2", "event_type": "Draft vendor created", "primary_reference": {"id": "vendor-2", "resource_name": "Vendor / Merchant", "url": "/vendors/vendor-2"}}
	]`)
	raw := []json.RawMessage{}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	got, err := flattenAuditEvents(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}
	if got[0].ID != "ev1" || got[1].ID != "ev2" {
		t.Fatalf("unexpected ids: %s, %s", got[0].ID, got[1].ID)
	}
}

// TestFlattenAuditEvents_NestedArray covers the spec-documented nested-array
// shape. Values are anonymized synthetic fixtures, not live audit data.
func TestFlattenAuditEvents_NestedArray(t *testing.T) {
	body := []byte(`[
		[
			{"id": "ev1", "event_type": "X", "primary_reference": {"id": "v1", "resource_name": "Vendor / Merchant", "url": "/v/1"}},
			{"id": "ev2", "event_type": "Y", "primary_reference": {"id": "v2", "resource_name": "Vendor / Merchant", "url": "/v/2"}}
		],
		[
			{"id": "ev3", "event_type": "Z", "primary_reference": {"id": "v3", "resource_name": "Vendor / Merchant", "url": "/v/3"}}
		]
	]`)
	raw := []json.RawMessage{}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	got, err := flattenAuditEvents(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d", len(got))
	}
	for i, want := range []string{"ev1", "ev2", "ev3"} {
		if got[i].ID != want {
			t.Fatalf("event %d: got %s, want %s", i, got[i].ID, want)
		}
	}
}

// TestFlattenAuditEvents_Mixed covers the defensively tolerated case of mixed
// flat-and-nested entries; both shapes flatten correctly.
func TestFlattenAuditEvents_Mixed(t *testing.T) {
	body := []byte(`[
		{"id": "ev1", "event_type": "X", "primary_reference": {"id": "v1", "resource_name": "Vendor / Merchant", "url": "/v/1"}},
		[
			{"id": "ev2", "event_type": "Y", "primary_reference": {"id": "v2", "resource_name": "Vendor / Merchant", "url": "/v/2"}}
		]
	]`)
	raw := []json.RawMessage{}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	got, err := flattenAuditEvents(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}
}

func TestFlattenAuditEvents_RejectsUnexpectedScalar(t *testing.T) {
	raw := []json.RawMessage{json.RawMessage(`"not-an-event"`)}
	if _, err := flattenAuditEvents(raw); err == nil {
		t.Fatal("expected scalar audit log entry to fail")
	}
}

func TestListAuditLogEventsAddsPageSizeToCursorPages(t *testing.T) {
	ctx := context.Background()
	var requests []*http.Request

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Clone(ctx))
		w.Header().Set("Content-Type", "application/json")
		switch len(requests) {
		case 1:
			_, _ = w.Write([]byte(`{"data":[],"page":{"next":"` + server.URL + `/developer/v1/audit-logs/events?start=cursor-1"}}`))
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

	req := &AuditLogEventsRequest{
		PageSize: 50,
	}
	_, nextPage, _, err := c.ListAuditLogEvents(ctx, req, "")
	if err != nil {
		t.Fatal(err)
	}
	if nextPage == "" {
		t.Fatal("expected first page to return pagination token")
	}
	if _, _, _, err := c.ListAuditLogEvents(ctx, req, nextPage); err != nil {
		t.Fatal(err)
	}

	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requests))
	}
	for i, request := range requests {
		query := request.URL.Query()
		if query.Get("page_size") != "50" {
			t.Fatalf("request %d page_size query = %q, want 50", i, query.Get("page_size"))
		}
	}
	if requests[0].URL.Query().Get("start") != "" {
		t.Fatalf("first request start = %q, want empty", requests[0].URL.Query().Get("start"))
	}
	if requests[1].URL.Query().Get("start") != "cursor-1" {
		t.Fatalf("second request start = %q, want cursor-1", requests[1].URL.Query().Get("start"))
	}
}
