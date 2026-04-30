package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/conductorone/baton-ramp/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// auditEventFeed implements connectorbuilder.EventFeed against Ramp's
// audit-log API. Filters to vendor-management events (vendors and
// agreements) and emits one ResourceChangeEvent per event for the
// affected resource.
//
// The platform's BatonFeedConsumerWorkflow polls ListEvents on a per-
// catalog cadence (typically 1h for read-only sources), checkpoints
// the cursor, and turns ResourceChangeEvents into targeted-sync tasks
// that re-fetch only the affected resources. No Ramp-specific work on
// the platform side; this connector just has to translate audit
// entries into v2.Events.
type auditEventFeed struct {
	client *client.Client
}

func newAuditEventFeed(c *client.Client) *auditEventFeed {
	return &auditEventFeed{client: c}
}

// EventFeedID is the stable feed identifier the platform uses to
// checkpoint cursors.
const EventFeedID = "vendor_audit_feed"

func (f *auditEventFeed) EventFeedMetadata(ctx context.Context) *v2.EventFeedMetadata {
	return v2.EventFeedMetadata_builder{
		Id: EventFeedID,
		SupportedEventTypes: []v2.EventType{
			v2.EventType_EVENT_TYPE_RESOURCE_CHANGE,
		},
	}.Build()
}

// eventPageToken is what we marshal into the StreamToken cursor between
// polls. NextPageToken is Ramp's opaque page.next; LastEventTime is the
// max event_time we've seen, used as a defensive lower bound on the next
// poll in case Ramp returns out-of-order events.
type eventPageToken struct {
	NextPageToken string    `json:"next_page_token,omitempty"`
	LastEventTime time.Time `json:"last_event_time,omitempty"`
}

func decodeEventPageToken(s string) (*eventPageToken, error) {
	t := &eventPageToken{}
	if s == "" {
		return t, nil
	}
	if err := json.Unmarshal([]byte(s), t); err != nil {
		return nil, fmt.Errorf("baton-ramp: failed to decode event page token: %w", err)
	}
	return t, nil
}

func encodeEventPageToken(t *eventPageToken) (string, error) {
	if t == nil {
		return "", nil
	}
	b, err := json.Marshal(t)
	if err != nil {
		return "", fmt.Errorf("baton-ramp: failed to encode event page token: %w", err)
	}
	return string(b), nil
}

// ListEvents fetches one page of audit events from Ramp, filters to the
// vendor-management subset, and returns ResourceChangeEvents for each.
func (f *auditEventFeed) ListEvents(
	ctx context.Context,
	earliestEvent *timestamppb.Timestamp,
	pToken *pagination.StreamToken,
) ([]*v2.Event, *pagination.StreamState, annotations.Annotations, error) {
	var annos annotations.Annotations

	cursor, err := decodeEventPageToken(pToken.Cursor)
	if err != nil {
		return nil, nil, annos, err
	}

	// On the very first call, earliestEvent is set by the platform to
	// the time of the last successful sync; events older than that have
	// already been incorporated. On subsequent calls cursor.NextPageToken
	// drives pagination through Ramp.
	earliest := time.Time{}
	if earliestEvent != nil {
		earliest = earliestEvent.AsTime()
	}
	if cursor.LastEventTime.After(earliest) {
		earliest = cursor.LastEventTime
	}

	auditEvents, nextPageToken, ratelimitData, err := f.client.ListAuditLogEvents(ctx, cursor.NextPageToken)
	annos.WithRateLimiting(ratelimitData)
	if err != nil {
		return nil, nil, annos, err
	}

	events := make([]*v2.Event, 0, len(auditEvents))
	maxEventTime := cursor.LastEventTime

	for _, ae := range auditEvents {
		eventTime, err := time.Parse(time.RFC3339, ae.EventTime)
		if err != nil {
			// Skip unparseable timestamps rather than fail the whole batch.
			continue
		}
		if eventTime.After(maxEventTime) {
			maxEventTime = eventTime
		}
		if !earliest.IsZero() && !eventTime.After(earliest) {
			// Already seen in a prior poll; defensive against out-of-
			// order delivery.
			continue
		}
		if shouldSkipAuditEvent(ae) {
			continue
		}
		ev := f.toResourceChangeEvent(ae, eventTime)
		if ev == nil {
			continue
		}
		events = append(events, ev)
	}

	state := &pagination.StreamState{
		HasMore: nextPageToken != "",
	}
	state.Cursor, err = encodeEventPageToken(&eventPageToken{
		NextPageToken: nextPageToken,
		LastEventTime: maxEventTime,
	})
	if err != nil {
		return nil, nil, annos, err
	}

	return events, state, annos, nil
}

// vendorManagementEventTypes is the set of audit event types we emit
// ResourceChangeEvents for. Filter is client-side because Ramp's
// audit-logs endpoint takes no documented event_type query param.
//
// String spelling and spacing (including the literal double space in two
// of the "Vendor management" entries) matches the Ramp spec exactly. Do
// not "fix" the double-space by trimming — it's the authoritative
// identifier.
var vendorManagementEventTypes = map[string]struct{}{
	// Vendor lifecycle.
	"Vendor management  vendor added to managed list":     {},
	"Vendor management  vendor removed from managed list": {},
	"Draft vendor created":                                 {},
	"Draft vendor published":                               {},
	"Merged vendors":                                       {},
	"Vendor placed on hold":                                {},
	"Vendor hold released":                                 {},
	"Vendor imported from erp":                             {},

	// Agreement lifecycle.
	"Vendor management agreement status changed":              {},
	"Vendor management edited agreement field":                {},
	"Vendor management agreement notification type switched":  {},
	"Vendor management agreement linked document":             {},
	"Vendor management agreement unlinked document":           {},
	"Vendor management agreement uploaded document":           {},
	"Vendor management agreement deleted document":            {},
	"Vendor management agreement linked purchase order":       {},
	"Vendor management agreement unlinked purchase order":     {},
	"Vendor management expansion request status changed":      {},
	"Generated renewal brief for contract":                    {},
	"Combined contracts with this contract":                   {},
	"Bill linked to contract":                                 {},
	"Bill unlinked from contract":                             {},
	// Skipped on purpose:
	//   "Vendor management agreement deleted" — deleted resources fail
	//   subsequent Get() calls during incremental sync. The next full
	//   sync's snapshot diff stamps deleted_at correctly without our
	//   help.
	//   "Draft vendor deleted" — same reasoning.
}

// shouldSkipAuditEvent returns true for events we don't translate.
func shouldSkipAuditEvent(ae *client.AuditLogEvent) bool {
	if ae == nil || ae.PrimaryReference == nil {
		return true
	}
	// We only care about events targeting a vendor / merchant.
	if ae.PrimaryReference.ResourceName != "Vendor / Merchant" {
		return true
	}
	if _, ok := vendorManagementEventTypes[ae.EventType]; !ok {
		return true
	}
	if ae.PrimaryReference.ID == "" {
		return true
	}
	return false
}

// toResourceChangeEvent maps an audit-log entry to a v2.Event. The
// PrimaryReference URL pattern tells us whether the affected resource
// is a vendor (`/vendors/<id>`) or an agreement (`/contracts/<id>`).
//
// In v1 we only sync vendor and vendor_agreement resource types, so we
// emit ResourceChangeEvents against those types. Agreement-type URLs
// produce vendor_agreement events; everything else (including vendors)
// produces vendor events.
func (f *auditEventFeed) toResourceChangeEvent(ae *client.AuditLogEvent, eventTime time.Time) *v2.Event {
	resourceType := "vendor"
	if isAgreementURL(ae.PrimaryReference.URL) {
		resourceType = "vendor_agreement"
	}
	return v2.Event_builder{
		Id:         ae.ID,
		OccurredAt: timestamppb.New(eventTime),
		ResourceChangeEvent: v2.ResourceChangeEvent_builder{
			ResourceId: v2.ResourceId_builder{
				ResourceType: resourceType,
				Resource:     ae.PrimaryReference.ID,
			}.Build(),
		}.Build(),
	}.Build()
}

// isAgreementURL recognises Ramp's contract page URLs. The audit log's
// primary_reference.url is relative-ish (path-only or full URL); both
// shapes contain "/contracts/" for agreement-targeted entries.
func isAgreementURL(url string) bool {
	return strings.Contains(url, "/contracts/")
}
