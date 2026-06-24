package client

import (
	"encoding/json"
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
