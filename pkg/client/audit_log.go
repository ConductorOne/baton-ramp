package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
)

const auditLogEventsPath = "audit-logs/events"

// ListAuditLogEvents fetches one page of audit-log events from
// GET /developer/v1/audit-logs/events.
//
// Ramp's spec types `data` as a doubly-nested array (`{...}[][]`); we
// decode raw and unwrap a single layer if present, returning a flat slice
// to callers. If the spec turns out to be wrong on the wire (single-array
// shape), the unmarshal fallback handles it.
//
// pagination is the opaque `page.next` cursor from a previous response, or
// "" for the first page.
//
// Required scope: audit_logs:read.
//
// https://docs.ramp.com/developer-api/v1/api/audit-logs
func (c *Client) ListAuditLogEvents(
	ctx context.Context,
	pagination string,
) ([]*AuditLogEvent, string, *v2.RateLimitDescription, error) {
	reqURL := pagination
	if reqURL == "" {
		var err error
		reqURL, err = c.newUnPaginatedURL(auditLogEventsPath)
		if err != nil {
			return nil, "", nil, err
		}
	}
	resp := &AuditLogEventsList{}
	ratelimitData, err := c.query(ctx, http.MethodGet, reqURL, resp)
	if err != nil {
		return nil, "", ratelimitData, fmt.Errorf("baton-ramp: error listing audit log events: %w", err)
	}

	events, err := flattenAuditEvents(resp.DataRaw)
	if err != nil {
		return nil, "", ratelimitData, err
	}
	return events, resp.Page.Next, ratelimitData, nil
}

// flattenAuditEvents tolerates both the spec-documented `data: T[][]` shape
// and a flat `data: T[]` shape. Each top-level entry is decoded based on its
// JSON token shape rather than by speculative unmarshal fallback.
func flattenAuditEvents(raw []json.RawMessage) ([]*AuditLogEvent, error) {
	out := make([]*AuditLogEvent, 0, len(raw))
	for _, item := range raw {
		trimmed := strings.TrimSpace(string(item))
		if trimmed == "" {
			return nil, fmt.Errorf("baton-ramp: failed to decode audit log entry: empty entry")
		}

		switch trimmed[0] {
		case '[':
			var batch []*AuditLogEvent
			if err := json.Unmarshal(item, &batch); err != nil {
				return nil, fmt.Errorf("baton-ramp: failed to decode audit log entry batch: %w", err)
			}
			out = append(out, batch...)
		case '{':
			ev := &AuditLogEvent{}
			if err := json.Unmarshal(item, ev); err != nil {
				return nil, fmt.Errorf("baton-ramp: failed to decode audit log entry: %w", err)
			}
			out = append(out, ev)
		default:
			return nil, fmt.Errorf("baton-ramp: failed to decode audit log entry: expected object or array")
		}
	}
	return out, nil
}
