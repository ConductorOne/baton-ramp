package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/conductorone/baton-ramp/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	// resourceNameVendorMerchant is Ramp's audit-log ResourceName for vendor
	// (a.k.a. merchant) records. Spelled exactly per the Ramp spec.
	resourceNameVendorMerchant = "Vendor / Merchant"

	// eventTypeAgreementStatusChanged is the Ramp audit event type emitted when
	// an agreement's status changes. Spelled exactly per the Ramp spec.
	eventTypeAgreementStatusChanged = "Vendor management agreement status changed"

	// eventTypeVendorAddedToManagedList is spelled exactly per the Ramp spec,
	// including the literal double space after "management".
	eventTypeVendorAddedToManagedList = "Vendor management  vendor added to managed list"
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
	client                  *client.Client
	vendorAgreementsEnabled bool
}

func newAuditEventFeed(c *client.Client, vendorAgreementsEnabled bool) *auditEventFeed {
	return &auditEventFeed{
		client:                  c,
		vendorAgreementsEnabled: vendorAgreementsEnabled,
	}
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
// max event_time we've seen. FloorEventTime is fixed at the start of a
// paginated stream so descending pages don't get filtered by page 1's max.
type eventPageToken struct {
	NextPageToken  string    `json:"next_page_token,omitempty"`
	LastEventTime  time.Time `json:"last_event_time,omitempty"`
	FloorEventTime time.Time `json:"floor_event_time,omitempty"`
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

	// On the first page of a poll, earliestEvent is set by the platform to
	// the time of the last successful sync; events older than that have
	// already been incorporated. Keep that floor stable while consuming
	// Ramp cursor pages, because Ramp may return audit pages newest-first.
	floor := time.Time{}
	if earliestEvent != nil {
		floor = earliestEvent.AsTime()
	}
	if cursor.NextPageToken != "" {
		if !cursor.FloorEventTime.IsZero() {
			floor = cursor.FloorEventTime
		}
	} else if cursor.LastEventTime.After(floor) {
		floor = cursor.LastEventTime
	}

	auditEvents, nextPageToken, ratelimitData, err := f.client.ListAuditLogEvents(ctx, cursor.NextPageToken)
	annos.WithRateLimiting(ratelimitData)
	if err != nil {
		return nil, nil, annos, err
	}

	events := make([]*v2.Event, 0, len(auditEvents))
	maxEventTime := cursor.LastEventTime
	pageHasParsedEvents := false
	pageDescending := true
	pageAllAtOrBeforeFloor := !floor.IsZero()
	var previousEventTime time.Time

	for _, ae := range auditEvents {
		eventTime, err := time.Parse(time.RFC3339, ae.EventTime)
		if err != nil {
			// Skip unparseable timestamps rather than fail the whole batch.
			ctxzap.Extract(ctx).Debug(
				"skipping audit event with unparseable event_time",
				zap.String("audit_event_id", ae.ID),
				zap.String("event_time", ae.EventTime),
				zap.Error(err),
			)
			pageAllAtOrBeforeFloor = false
			continue
		}
		if pageHasParsedEvents && eventTime.After(previousEventTime) {
			pageDescending = false
		}
		pageHasParsedEvents = true
		previousEventTime = eventTime
		if floor.IsZero() || eventTime.After(floor) {
			pageAllAtOrBeforeFloor = false
		}
		if eventTime.After(maxEventTime) {
			maxEventTime = eventTime
		}
		if !floor.IsZero() && !eventTime.After(floor) {
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

	effectiveNextPageToken := nextPageToken
	if nextPageToken != "" && pageHasParsedEvents && pageDescending && pageAllAtOrBeforeFloor {
		// Ramp audit pages are commonly newest-first. Once a descending page
		// is entirely at or before the floor, later cursor pages are older and
		// cannot produce new ResourceChangeEvents for this poll.
		effectiveNextPageToken = ""
	}

	state := &pagination.StreamState{
		HasMore: effectiveNextPageToken != "",
	}
	nextCursor := &eventPageToken{
		NextPageToken: effectiveNextPageToken,
		LastEventTime: maxEventTime,
	}
	if effectiveNextPageToken != "" {
		nextCursor.FloorEventTime = floor
	}
	state.Cursor, err = encodeEventPageToken(nextCursor)
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
	eventTypeVendorAddedToManagedList:                     {},
	"Vendor management  vendor removed from managed list": {},
	"Draft vendor created":                                {},
	"Draft vendor published":                              {},
	"Merged vendors":                                      {},
	"Vendor placed on hold":                               {},
	"Vendor hold released":                                {},
	"Vendor imported from erp":                            {},

	// Agreement lifecycle.
	eventTypeAgreementStatusChanged:                          {},
	"Vendor management edited agreement field":               {},
	"Vendor management agreement notification type switched": {},
	"Vendor management agreement linked document":            {},
	"Vendor management agreement unlinked document":          {},
	"Vendor management agreement uploaded document":          {},
	"Vendor management agreement deleted document":           {},
	"Vendor management agreement linked purchase order":      {},
	"Vendor management agreement unlinked purchase order":    {},
	"Vendor management expansion request status changed":     {},
	"Generated renewal brief for contract":                   {},
	"Combined contracts with this contract":                  {},
	"Bill linked to contract":                                {},
	"Bill unlinked from contract":                            {},
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
	if ae.PrimaryReference.ResourceName != resourceNameVendorMerchant {
		return true
	}
	if _, ok := vendorManagementEventTypes[ae.EventType]; !ok {
		return true
	}
	_, resourceID := auditEventResourceTarget(ae.PrimaryReference)
	return resourceID == ""
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
	resourceType, resourceID := auditEventResourceTarget(ae.PrimaryReference)
	if resourceID == "" {
		return nil
	}
	if resourceType == vendorAgreementResourceTypeID {
		if !f.vendorAgreementsEnabled {
			return nil
		}
	}
	return v2.Event_builder{
		Id:         ae.ID,
		OccurredAt: timestamppb.New(eventTime),
		ResourceChangeEvent: v2.ResourceChangeEvent_builder{
			ResourceId: v2.ResourceId_builder{
				ResourceType: resourceType,
				Resource:     resourceID,
			}.Build(),
		}.Build(),
	}.Build()
}

func auditEventResourceTarget(ref *client.AuditLogReference) (string, string) {
	if ref == nil {
		return "", ""
	}
	if isAgreementURL(ref.URL) {
		return vendorAgreementResourceTypeID, agreementIDFromURL(ref.URL)
	}
	if ref.ID == "" {
		return "", ""
	}
	return vendorResourceTypeID, ref.ID
}

// isAgreementURL recognises Ramp's contract page URLs. The audit log's
// primary_reference.url is relative-ish (path-only or full URL); both
// shapes contain "/contracts/" for agreement-targeted entries.
func isAgreementURL(refURL string) bool {
	return strings.Contains(refURL, "/contracts/")
}

func agreementIDFromURL(refURL string) string {
	const contractPath = "/contracts/"
	idx := strings.Index(refURL, contractPath)
	if idx == -1 {
		return ""
	}
	rest := refURL[idx+len(contractPath):]
	if rest == "" {
		return ""
	}
	if cut := strings.IndexAny(rest, "/?#"); cut >= 0 {
		rest = rest[:cut]
	}
	if rest == "" {
		return ""
	}
	id, err := url.PathUnescape(rest)
	if err != nil {
		return rest
	}
	return id
}
