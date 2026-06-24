package connector

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/conductorone/baton-ramp/pkg/client"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"google.golang.org/protobuf/types/known/timestamppb"
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
			name: "agreement id from url",
			in: &client.AuditLogEvent{
				EventType: "Vendor management agreement status changed",
				PrimaryReference: &client.AuditLogReference{
					ResourceName: "Vendor / Merchant",
					ID:           "",
					URL:          "/contracts/agreement-id",
				},
			},
			want: false,
		},
		{
			name: "malformed agreement url",
			in: &client.AuditLogEvent{
				EventType: "Vendor management agreement status changed",
				PrimaryReference: &client.AuditLogReference{
					ResourceName: "Vendor / Merchant",
					ID:           "vendor-id",
					URL:          "/contracts/",
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
				EventType: eventTypeVendorAddedToManagedList,
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

func TestAuditEventFeedMapsAgreementEventsToContractURLID(t *testing.T) {
	eventTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	agreementEvent := &client.AuditLogEvent{
		ID:        "event-1",
		EventType: "Vendor management agreement status changed",
		PrimaryReference: &client.AuditLogReference{
			ResourceName: resourceNameVendorMerchant,
			ID:           "vendor-1",
			URL:          "https://app.ramp.com/contracts/agreement-1?tab=activity",
		},
	}

	feed := newAuditEventFeed(nil, true)
	got := feed.toResourceChangeEvent(agreementEvent, eventTime)
	if got == nil {
		t.Fatal("expected agreement event to be emitted")
	}
	resourceID := got.GetResourceChangeEvent().GetResourceId()
	if resourceID.GetResourceType() != vendorAgreementResourceTypeID {
		t.Fatalf("expected %s resource type, got %q", vendorAgreementResourceTypeID, resourceID.GetResourceType())
	}
	if resourceID.GetResource() != "agreement-1" {
		t.Fatalf("expected agreement id from URL, got %q", resourceID.GetResource())
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

func TestAuditEventFeedListEventsKeepsFloorStableAcrossDescendingPages(t *testing.T) {
	ctx := context.Background()
	earliest := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	firstPageTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	secondPageTime := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)

	var requests []string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")

		switch len(requests) {
		case 1:
			_, _ = w.Write([]byte(auditEventListResponse(
				"event-1",
				firstPageTime,
				"vendor-1",
				"/vendors/vendor-1",
				server.URL+"/developer/v1/audit-logs/events?start=cursor-1",
			)))
		case 2:
			if r.URL.Query().Get("start") != "cursor-1" {
				t.Fatalf("expected second request to use Ramp cursor URL, got %s", r.URL.RequestURI())
			}
			_, _ = w.Write([]byte(auditEventListResponse(
				"event-2",
				secondPageTime,
				"vendor-2",
				"/vendors/vendor-2",
				"",
			)))
		default:
			t.Fatalf("unexpected request %d: %s", len(requests), r.URL.RequestURI())
		}
	}))
	defer server.Close()

	c, err := client.New(ctx, client.Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	feed := newAuditEventFeed(c, true)

	firstEvents, firstState, _, err := feed.ListEvents(ctx, timestamppb.New(earliest), &pagination.StreamToken{})
	if err != nil {
		t.Fatal(err)
	}
	if len(firstEvents) != 1 || firstEvents[0].GetId() != "event-1" {
		t.Fatalf("expected first page event, got %+v", firstEvents)
	}
	if firstState == nil || !firstState.HasMore {
		t.Fatalf("expected first state to have more pages, got %+v", firstState)
	}

	firstCursor, err := decodeEventPageToken(firstState.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	if firstCursor.NextPageToken == "" {
		t.Fatal("expected first cursor to preserve Ramp page token")
	}
	if !firstCursor.FloorEventTime.Equal(earliest) {
		t.Fatalf("expected floor %s, got %s", earliest, firstCursor.FloorEventTime)
	}
	if !firstCursor.LastEventTime.Equal(firstPageTime) {
		t.Fatalf("expected max event time %s, got %s", firstPageTime, firstCursor.LastEventTime)
	}

	secondEvents, secondState, _, err := feed.ListEvents(ctx, timestamppb.New(earliest), &pagination.StreamToken{Cursor: firstState.Cursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(secondEvents) != 1 || secondEvents[0].GetId() != "event-2" {
		t.Fatalf("expected second page event not to be filtered, got %+v", secondEvents)
	}
	if secondState == nil || secondState.HasMore {
		t.Fatalf("expected second state to end pagination, got %+v", secondState)
	}

	secondCursor, err := decodeEventPageToken(secondState.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	if secondCursor.NextPageToken != "" {
		t.Fatalf("expected final cursor to clear Ramp page token, got %q", secondCursor.NextPageToken)
	}
	if !secondCursor.LastEventTime.Equal(firstPageTime) {
		t.Fatalf("expected final high-water mark %s, got %s", firstPageTime, secondCursor.LastEventTime)
	}
}

func TestAuditEventFeedStopsDescendingPaginationAtFloor(t *testing.T) {
	ctx := context.Background()
	earliest := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	firstPageTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	secondPageTime := time.Date(2026, 1, 1, 7, 0, 0, 0, time.UTC)

	var requests []string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")

		switch len(requests) {
		case 1:
			_, _ = w.Write([]byte(auditEventListResponse(
				"event-1",
				firstPageTime,
				"vendor-1",
				"/vendors/vendor-1",
				server.URL+"/developer/v1/audit-logs/events?start=cursor-1",
			)))
		case 2:
			_, _ = w.Write([]byte(auditEventListResponse(
				"event-old",
				secondPageTime,
				"vendor-old",
				"/vendors/vendor-old",
				server.URL+"/developer/v1/audit-logs/events?start=cursor-2",
			)))
		default:
			t.Fatalf("unexpected request %d: %s", len(requests), r.URL.RequestURI())
		}
	}))
	defer server.Close()

	c, err := client.New(ctx, client.Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	feed := newAuditEventFeed(c, true)

	_, firstState, _, err := feed.ListEvents(ctx, timestamppb.New(earliest), &pagination.StreamToken{})
	if err != nil {
		t.Fatal(err)
	}
	if firstState == nil || !firstState.HasMore {
		t.Fatalf("expected first state to have more pages, got %+v", firstState)
	}

	secondEvents, secondState, _, err := feed.ListEvents(ctx, timestamppb.New(earliest), &pagination.StreamToken{Cursor: firstState.Cursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(secondEvents) != 0 {
		t.Fatalf("expected old page to emit no events, got %+v", secondEvents)
	}
	if secondState == nil || secondState.HasMore {
		t.Fatalf("expected old descending page to terminate pagination, got %+v", secondState)
	}
	if len(requests) != 2 {
		t.Fatalf("expected early stop after 2 requests, got %d", len(requests))
	}
}

func TestAuditEventFeedSkipsAgreementEventsWhenAgreementsDisabled(t *testing.T) {
	eventTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	agreementEvent := &client.AuditLogEvent{
		ID:        "event-1",
		EventType: "Vendor management agreement status changed",
		PrimaryReference: &client.AuditLogReference{
			ResourceName: resourceNameVendorMerchant,
			ID:           "agreement-1",
			URL:          "/contracts/agreement-1",
		},
	}
	vendorEvent := &client.AuditLogEvent{
		ID:        "event-2",
		EventType: eventTypeVendorAddedToManagedList,
		PrimaryReference: &client.AuditLogReference{
			ResourceName: resourceNameVendorMerchant,
			ID:           "vendor-1",
			URL:          "/vendors/vendor-1",
		},
	}

	feed := newAuditEventFeed(nil, false)
	if got := feed.toResourceChangeEvent(agreementEvent, eventTime); got != nil {
		t.Fatalf("expected disabled agreement event to be skipped, got %+v", got)
	}
	if got := feed.toResourceChangeEvent(vendorEvent, eventTime); got == nil {
		t.Fatal("expected vendor event to be emitted")
	}
}

func auditEventListResponse(eventID string, eventTime time.Time, referenceID string, referenceURL string, next string) string {
	return fmt.Sprintf(
		`{"data":[{"id":%q,"event_type":%q,"event_time":%q,"primary_reference":{"id":%q,"resource_name":%q,"url":%q}}],"page":{"next":%q}}`,
		eventID,
		eventTypeVendorAddedToManagedList,
		eventTime.Format(time.RFC3339),
		referenceID,
		resourceNameVendorMerchant,
		referenceURL,
		next,
	)
}
