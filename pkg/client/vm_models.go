package client

import (
	"bytes"
	"encoding/json"
)

// vm_models.go: types used by the vendor-management surface
// (agreements and audit-log feed). Kept separate from the core users/vendors
// models so the diff is easy to audit.

// AgreementContractOwner is a Ramp user designated as a contract owner
// on a vendor agreement. The id is a Ramp user ID and matches a row in
// /developer/v1/users (see UsersList).
type AgreementContractOwner struct {
	ID                string `json:"id"`
	FirstName         string `json:"first_name"`
	LastName          string `json:"last_name"`
	FullName          string `json:"full_name"`
	Email             string `json:"email"`
	ProfilePictureURL string `json:"profile_picture_url"`
}

// AgreementPayee is the vendor side of an agreement; carries the
// business_vendor.uuid that joins back to /developer/v1/vendors.
type AgreementPayee struct {
	UUID           string                        `json:"uuid"`
	Name           string                        `json:"name"`
	DBAName        string                        `json:"dba_name"`
	Website        string                        `json:"website"`
	ImageURL       string                        `json:"image_url"`
	BusinessVendor *AgreementPayeeBusinessVendor `json:"business_vendor"`
}

type AgreementPayeeBusinessVendor struct {
	UUID                           string `json:"uuid"`
	VendorTrackingCategoryOptionID int    `json:"vendor_tracking_category_option_id"`
}

// AgreementDaysRemaining is Ramp's pre-computed countdown for the agreement.
type AgreementDaysRemaining struct {
	NumDaysUntilEnd *int    `json:"num_days_until_end"`
	IsExpired       *bool   `json:"is_expired"`
	IsOverdue       *bool   `json:"is_overdue"`
	EndDateUsed     *string `json:"end_date_used"`
}

// VendorAgreementListItem is the shape returned by
// POST /developer/v1/vendors/agreements (list/search).
//
// The single-fetch GET /developer/v1/vendors/agreements/{id} returns a
// superset of these fields plus a richer payee, available_actions, and
// purchase_orders. We model only the fields we read.
type VendorAgreementListItem struct {
	ID                  string                   `json:"id"`
	Name                string                   `json:"name"`
	Description         string                   `json:"description"`
	StartDate           string                   `json:"start_date"`
	EndDate             string                   `json:"end_date"`
	LastDateToTerminate string                   `json:"last_date_to_terminate"`
	AutoRenewal         bool                     `json:"auto_renewal"`
	RenewalStatus       string                   `json:"renewal_status"`
	IsUpForRenewal      bool                     `json:"is_up_for_renewal"`
	IsSnoozed           bool                     `json:"is_snoozed"`
	NotificationsOn     bool                     `json:"notifications_on"`
	Currency            string                   `json:"currency"`
	TotalValue          *Money                   `json:"total_value"`
	PayeeID             string                   `json:"payee_id"`
	PayeeName           string                   `json:"payee_name"`
	Logo                string                   `json:"logo"`
	ContractOwners      []AgreementContractOwner `json:"contract_owners"`
	DaysRemaining       *AgreementDaysRemaining  `json:"days_remaining"`
	CreatedAt           string                   `json:"created_at"`
	UpdatedAt           string                   `json:"updated_at"`
	DeletedAt           string                   `json:"deleted_at"`
}

// VendorAgreement is the single-fetch response from
// GET /developer/v1/vendors/agreements/{id}.
//
// `LineItems` is typed as `[]map[string]any` because the Ramp spec types
// it as `Record<string, unknown>[]` (opaque). Connectors that want to
// surface line items will need a follow-up that probes a real tenant
// to nail down the shape.
type VendorAgreement struct {
	ID                  string                   `json:"id"`
	Name                string                   `json:"name"`
	Description         string                   `json:"description"`
	StartDate           string                   `json:"start_date"`
	EndDate             string                   `json:"end_date"`
	LastDateToTerminate string                   `json:"last_date_to_terminate"`
	AutoRenewal         bool                     `json:"auto_renewal"`
	RenewalStatus       string                   `json:"renewal_status"`
	IsActive            bool                     `json:"is_active"`
	IsUpForRenewal      bool                     `json:"is_up_for_renewal"`
	NotificationsOn     bool                     `json:"notifications_on"`
	Currency            string                   `json:"currency"`
	TotalValue          *Money                   `json:"total_value"`
	ContractOwners      []AgreementContractOwner `json:"contract_owners"`
	Payee               *AgreementPayee          `json:"payee"`
	DaysRemaining       *AgreementDaysRemaining  `json:"days_remaining"`
	LineItems           []map[string]any         `json:"line_items"`
	CreatedAt           string                   `json:"created_at"`
	UpdatedAt           string                   `json:"updated_at"`
	ArchivedAt          string                   `json:"archived_at"`
	DeletedAt           string                   `json:"deleted_at"`
}

// VendorAgreementsList is the wire response for the list endpoint.
type VendorAgreementsList struct {
	Page Page                       `json:"page"`
	Data []*VendorAgreementListItem `json:"data"`
}

func (l *VendorAgreementsList) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil
	}
	if trimmed[0] == '[' {
		var agreements []*VendorAgreementListItem
		if err := json.Unmarshal(trimmed, &agreements); err != nil {
			return err
		}
		l.Page = Page{}
		l.Data = agreements
		return nil
	}

	type vendorAgreementsList VendorAgreementsList
	var out vendorAgreementsList
	if err := json.Unmarshal(trimmed, &out); err != nil {
		return err
	}
	*l = VendorAgreementsList(out)
	return nil
}

// VendorAgreementsResponse is the connector-friendly list response.
type VendorAgreementsResponse struct {
	Agreements []*VendorAgreementListItem
	Pagination string
}

// VendorAgreementsListRequest is the body for
// POST /developer/v1/vendors/agreements. Only the fields we send are
// modeled; the API accepts many more (see Ramp docs).
type VendorAgreementsListRequest struct {
	IncludeArchived bool   `json:"include_archived,omitempty"`
	IsActive        *bool  `json:"is_active,omitempty"`
	PageSize        int    `json:"page_size,omitempty"`
	Start           string `json:"start,omitempty"`
}

// AuditLogEvent is the per-event shape from
// GET /developer/v1/audit-logs/events.
type AuditLogEvent struct {
	ID                string                `json:"id"`
	EventType         string                `json:"event_type"`
	EventTime         string                `json:"event_time"`
	ActorType         string                `json:"actor_type"`
	ActorID           string                `json:"actor_id"`
	AdditionalDetails string                `json:"additional_details"`
	PrimaryReference  *AuditLogReference    `json:"primary_reference"`
	EventDetails      *AuditLogEventDetails `json:"event_details"`
}

type AuditLogReference struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	ResourceName string `json:"resource_name"`
	URL          string `json:"url"`
}

type AuditLogEventDetails struct {
	References []AuditLogReference `json:"references"`
}

// AuditLogEventsList wraps the audit-log API's response. The Ramp spec
// types `data` as a doubly-nested array (`{...}[][]`); we mirror that
// shape so json decoding doesn't fail. ListAuditLogEvents flattens it
// for callers and tolerates a single-array shape if the spec is wrong.
type AuditLogEventsList struct {
	Page    Page              `json:"page"`
	DataRaw []json.RawMessage `json:"data"`
}
