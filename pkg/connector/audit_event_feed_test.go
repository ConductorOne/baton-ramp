package connector

import (
	"testing"

	"github.com/conductorone/baton-ramp/pkg/client"
)

func TestShouldSkipAuditEvent(t *testing.T) {
	cases := []struct {
		name string
		in   *client.AuditLogEvent
		want bool
	}{
		{
			name: "nil",
			in:   nil,
			want: true,
		},
		{
			name: "no primary reference",
			in:   &client.AuditLogEvent{EventType: "Vendor management agreement status changed"},
			want: true,
		},
		{
			name: "wrong resource_name",
			in: &client.AuditLogEvent{
				EventType: "Vendor management agreement status changed",
				PrimaryReference: &client.AuditLogReference{
					ResourceName: "Transaction",
					ID:           "abc",
				},
			},
			want: true,
		},
		{
			name: "unhandled event type",
			in: &client.AuditLogEvent{
				EventType: "User created",
				PrimaryReference: &client.AuditLogReference{
					ResourceName: "Vendor / Merchant",
					ID:           "v1",
				},
			},
			want: true,
		},
		{
			name: "missing id",
			in: &client.AuditLogEvent{
				EventType: "Vendor management agreement status changed",
				PrimaryReference: &client.AuditLogReference{
					ResourceName: "Vendor / Merchant",
					ID:           "",
				},
			},
			want: true,
		},
		{
			name: "happy path: agreement status changed",
			in: &client.AuditLogEvent{
				EventType: "Vendor management agreement status changed",
				PrimaryReference: &client.AuditLogReference{
					ResourceName: "Vendor / Merchant",
					ID:           "agreement-id",
					URL:          "/contracts/agreement-id",
				},
			},
			want: false,
		},
		{
			name: "happy path: vendor added to managed list (note double space)",
			in: &client.AuditLogEvent{
				EventType: "Vendor management  vendor added to managed list",
				PrimaryReference: &client.AuditLogReference{
					ResourceName: "Vendor / Merchant",
					ID:           "vendor-id",
					URL:          "/vendors/vendor-id",
				},
			},
			want: false,
		},
		{
			name: "intentionally skipped: agreement deleted",
			in: &client.AuditLogEvent{
				EventType: "Vendor management agreement deleted",
				PrimaryReference: &client.AuditLogReference{
					ResourceName: "Vendor / Merchant",
					ID:           "agreement-id",
					URL:          "/contracts/agreement-id",
				},
			},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldSkipAuditEvent(tc.in); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEventPageTokenRoundtrip(t *testing.T) {
	in := &eventPageToken{
		NextPageToken: "abc123",
	}
	encoded, err := encodeEventPageToken(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := decodeEventPageToken(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if out.NextPageToken != "abc123" {
		t.Fatalf("got %s", out.NextPageToken)
	}
}

func TestDecodeEventPageToken_Empty(t *testing.T) {
	out, err := decodeEventPageToken("")
	if err != nil {
		t.Fatal(err)
	}
	if out.NextPageToken != "" || !out.LastEventTime.IsZero() {
		t.Fatalf("expected zero token, got %+v", out)
	}
}
