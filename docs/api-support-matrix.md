# Ramp API support matrix

Source snapshot: Ramp Developer API OpenAPI and plaintext API reference fetched on 2026-06-24 from:

- https://docs.ramp.com/openapi/developer-api.json
- https://docs.ramp.com/llms-api.txt

This matrix compares Ramp API fields to the current `baton-ramp` implementation and the legacy C1 Ramp v1 connector where it explains migration behavior.

## Investigation summary

The reported missing `Admin` and `IT Admin` entitlements do not match the current `baton-ramp` role builder or the C1-vendored v2 snapshot: both define `BUSINESS_ADMIN` (`Admin`) and `IT_ADMIN` (`IT Admin`) as role resources with a `member` entitlement.

The legacy C1 v1 connector hard-coded these five role resources:

- `BUSINESS_ADMIN`
- `BUSINESS_USER`
- `BUSINESS_OWNER`
- `BUSINESS_BOOKKEEPER`
- `IT_ADMIN`

The v2 connector also models roles as static resources and derives grants by listing users and comparing `user.role` to each role ID. There is no Ramp roles endpoint involved. If `Admin` or `IT Admin` disappear in C1 while using a build that contains these role constants, the likely causes are:

- The customer is running an older or different v2 artifact than the code inspected here.
- The sync is not receiving users whose `role` is `BUSINESS_ADMIN` or `IT_ADMIN` from `GET /developer/v1/users`.
- C1 UI/reporting is showing only entitlements with grants, even though the role resources and entitlements are defined by the connector.

The current Ramp user list/fetch role enum has expanded beyond the original v1 set. `baton-ramp` now includes all role values returned by `GET /developer/v1/users` and `GET /developer/v1/users/{user_id}`. Create/update roles remain intentionally narrower because Ramp's create/update request schemas accept only the non-`UNBUNDLED_*` set.

## Endpoint support

| Ramp API surface | Used by connector | Support | Notes |
|---|---:|---|---|
| `GET /developer/v1/users` | Yes | Full for users and role grants | Lists users, builds user resources, and derives role grants from `role`. |
| `GET /developer/v1/users/{user_id}` | No | Missing | Not needed for sync because list returns the same user fields used by the connector. |
| `POST /developer/v1/users/deferred` | Yes | Partial | Supports account creation with `email`, `first_name`, `last_name`, and `role`. Other create fields are not exposed in the account creation schema. |
| `GET /developer/v1/users/deferred/status/{task_id}` | No | Missing | Connector returns action-required after invite; it does not poll deferred task status. |
| `PATCH /developer/v1/users/{user_id}` | No | Missing | Role/manager/location/department updates are not implemented. |
| `PATCH /developer/v1/users/{user_id}/deactivate` | Yes | Full | Exposed as `disable_user` action. |
| `PATCH /developer/v1/users/{user_id}/reactivate` | Yes | Full | Exposed as `enable_user` action. |
| `POST /developer/v1/users/{user_id}/invite` | No | Missing | Draft invite lifecycle is not implemented. |
| `GET /developer/v1/vendors` | Yes | Partial | Lists vendor resources, emits VendorTrait identity/spend fields, and caches `vendor_owner_id` for owner grants. |
| `GET /developer/v1/vendors/{vendor_id}` | Yes | Partial | Supports targeted sync and grant/revoke idempotency fallback. |
| `PATCH /developer/v1/vendors/{vendor_id}` | Yes | Partial / undocumented | Existing connector code sends `vendor_owner_id` to set or clear owner, but the current public PATCH schema does not list `vendor_owner_id`. Other mutable vendor fields are not managed. |
| `POST /developer/v1/vendors/agreements` | Yes | Partial | Opt-in `vendor_agreement` resource type. Lists agreements, emits VendorTrait and VendorAgreementTrait, and caches contract owners for grants. Requires `vendors:read`. |
| `GET /developer/v1/vendors/agreements/{agreement_id}` | Yes | Partial | Supports targeted sync and grant fallback. Requires `vendors:read`. |
| `GET /developer/v1/audit-logs/events` | Optional | Partial | Enabled only with `audit-log-events`; filters Ramp vendor-management events into vendor and vendor-agreement change events. Requires `audit_logs:read`. |
| `GET /developer/v1/business` | No | Missing | Not called. The connector does not require `business:read`; `source_business_id` remains empty. |
| Other vendor endpoints | No | Missing | Contacts, bank accounts, credits, children, and document flows are out of scope. |

## User response fields

Fields from `GET /developer/v1/users` and `GET /developer/v1/users/{user_id}`.

| Ramp field | Type / enum | Connector support | Code path | Notes |
|---|---|---|---|---|
| `id` | string | Full | `pkg/client/models.go`, `pkg/connector/users.go` | Stable user resource ID and grant principal ID. |
| `email` | string | Full | `pkg/client/models.go`, `pkg/connector/users.go` | User email and login. |
| `first_name` | string | Partial | `pkg/client/models.go`, `pkg/connector/users.go` | Used with `last_name` for display name only. |
| `last_name` | string | Partial | `pkg/client/models.go`, `pkg/connector/users.go` | Used with `first_name` for display name only. |
| `status` | enum | Full | `pkg/client/models.go`, `pkg/connector/users.go` | `USER_ACTIVE` and `USER_ONBOARDING` map to enabled; all other statuses map to disabled. |
| `role` | enum | Full | `pkg/client/models.go`, `pkg/connector/roles.go` | Used to derive role membership grants. |
| `business_id` | string | Missing | n/a | Not modeled. |
| `custom_fields` | array | Missing | n/a | Not modeled. |
| `department_id` | string | Missing | n/a | Not modeled or provisioned. |
| `employee_id` | string | Missing | n/a | Not modeled. |
| `entity_id` | string | Missing | n/a | Not modeled. |
| `is_manager` | boolean | Missing | n/a | Not modeled. |
| `location_id` | string | Missing | n/a | Not modeled or provisioned. |
| `manager_id` | string | Missing | n/a | Not modeled. |
| `phone` | string | Missing | n/a | Not modeled. |
| `scheduled_deactivation_date` | date string | Missing | n/a | Not modeled. |
| `scheduled_invitation_date` | date string | Missing | n/a | Not modeled. |

## Role enum support

Roles are not fetched from a Ramp roles endpoint. The connector creates static role resources and compares each user's `role` field to the role resource ID without the `role:` prefix.

| Ramp read role value | Display name | Legacy C1 v1 | `baton-ramp` sync | Create user support |
|---|---|---:|---:|---:|
| `AUDITOR` | Auditor | No | Yes | Yes |
| `BUSINESS_ADMIN` | Admin | Yes | Yes | Yes |
| `BUSINESS_BOOKKEEPER` | Bookkeeper | Yes | Yes | Yes |
| `BUSINESS_OWNER` | Owner | Yes | Yes | No |
| `BUSINESS_USER` | User | Yes | Yes | Yes |
| `GUEST_USER` | Guest | No | Yes | Yes |
| `IT_ADMIN` | IT Admin | Yes | Yes | Yes |
| `UNBUNDLED_ADMIN` | Unbundled Admin | No | Yes | No |
| `UNBUNDLED_BOOKKEEPER` | Unbundled Bookkeeper | No | Yes | No |
| `UNBUNDLED_OWNER` | Unbundled Owner | No | Yes | No |
| `UNBUNDLED_USER` | Unbundled User | No | Yes | No |

## User create request fields

Fields from `POST /developer/v1/users/deferred`.

| Ramp field | Type / enum | Connector support | Code path | Notes |
|---|---|---|---|---|
| `email` | string | Full | `pkg/connector/connector.go`, `pkg/connector/users.go` | Required account creation field. |
| `first_name` | string | Full | `pkg/connector/connector.go`, `pkg/connector/users.go` | Required account creation field. |
| `last_name` | string | Full | `pkg/connector/connector.go`, `pkg/connector/users.go` | Required account creation field. |
| `role` | enum | Full | `pkg/connector/connector.go`, `pkg/connector/users.go` | Required; accepts API-invitable roles only. Ramp can return `BUSINESS_OWNER` and `UNBUNDLED_*` on reads, but those values are not accepted for account creation. |
| `idempotency_key` | string | Internal | `pkg/client/users.go` | Generated automatically when not supplied. |
| `department_id` | string | Missing | n/a | Not exposed in C1 account creation schema. |
| `direct_manager_id` | string | Missing | n/a | Not exposed in C1 account creation schema. |
| `is_draft` | boolean | Missing | n/a | Not exposed in C1 account creation schema. |
| `is_manager` | boolean | Missing | n/a | Not exposed in C1 account creation schema. |
| `location_id` | string | Missing | n/a | Not exposed in C1 account creation schema. |
| `scheduled_deactivation_date` | date string | Missing | n/a | Not exposed in C1 account creation schema. |

## User update request fields

Fields from `PATCH /developer/v1/users/{user_id}`.

| Ramp field | Type / enum | Connector support | Code path | Notes |
|---|---|---|---|---|
| `auto_promote` | boolean | Missing | n/a | User update is not implemented. |
| `department_id` | string | Missing | n/a | User update is not implemented. |
| `direct_manager_id` | string | Missing | n/a | User update is not implemented. |
| `first_name` | string | Missing | n/a | User update is not implemented. |
| `is_manager` | boolean | Missing | n/a | User update is not implemented. |
| `last_name` | string | Missing | n/a | User update is not implemented. |
| `location_id` | string | Missing | n/a | User update is not implemented. |
| `role` | enum | Missing | n/a | Role reassignment provisioning is not implemented. |
| `scheduled_deactivation_date` | date string | Missing | n/a | User update is not implemented. |

## Vendor fields

Fields from `GET /developer/v1/vendors` and `GET /developer/v1/vendors/{vendor_id}`.

| Ramp field | Type / enum | Connector support | Code path | Notes |
|---|---|---|---|---|
| `id` | string | Full | `pkg/client/models.go`, `pkg/connector/vendors.go` | Stable vendor resource ID. |
| `name` | string | Full | `pkg/client/models.go`, `pkg/connector/vendors.go` | Vendor display name. |
| `vendor_owner_id` | string | Full | `pkg/client/models.go`, `pkg/connector/vendors.go` | Synced as `owner` entitlement grant; grant/revoke sets or clears it. |
| `name_legal` | string | Full | `pkg/client/models.go`, `pkg/connector/vendors.go` | Used as VendorTrait `vendor_name` when present; `name` becomes DBA/trade name. |
| `external_vendor_id` | string | Full | `pkg/client/models.go`, `pkg/connector/vendors.go` | Emitted as VendorTrait `external_vendor_id` when present. |
| `default_entity_id` | string | Full | `pkg/client/models.go`, `pkg/connector/vendors.go` | Emitted as VendorTrait `source_entity_id` when present. |
| `total_spend_last_30_days` | money object | Full | `pkg/client/models.go`, `pkg/connector/vendors.go` | Emitted as VendorTrait trailing 30-day spend. |
| `total_spend_last_365_days` | money object | Full | `pkg/client/models.go`, `pkg/connector/vendors.go` | Emitted as VendorTrait trailing 365-day spend. |
| `total_spend_ytd` | money object | Full | `pkg/client/models.go`, `pkg/connector/vendors.go` | Emitted as VendorTrait year-to-date spend. |
| `is_active` | boolean | Client only | `pkg/client/models.go` | Parsed but not exposed on the resource. |
| `accounting_vendor_remote_id` | string | Client only | `pkg/client/models.go` | Parsed but not exposed on the resource. |
| `address` | object | Missing | n/a | Not modeled. |
| `addresses` | array | Missing | n/a | Not modeled. |
| `billing_frequency` | enum | Client only | `pkg/client/models.go` | Parsed but not exposed on the resource. |
| `contacts` | array | Client only | `pkg/client/models.go` | Parsed but not exposed on the resource. |
| `country` | string | Client only | `pkg/client/models.go` | Parsed but not exposed on the resource. |
| `created_at` | date string | Client only | `pkg/client/models.go` | Parsed but not exposed on the resource. |
| `custom_form_collection_response` | object | Missing | n/a | Returned by fetch detail; not modeled. |
| `custom_record_fields` | array | Missing | n/a | Returned by fetch detail; not modeled. |
| `default_payment_method` | object | Missing | n/a | Not modeled. |
| `description` | string | Client only | `pkg/client/models.go` | Parsed but not exposed on the resource. |
| `federal_tax_classification` | enum | Missing | n/a | Not modeled. |
| `is_deletable` | boolean | Client only | `pkg/client/models.go` | Parsed but not exposed on the resource. |
| `merchant_id` | string | Client only | `pkg/client/models.go` | Parsed but not exposed on the resource. |
| `parent_vendor_id` | string | Missing | n/a | Not modeled. |
| `sk_category_id` | number | Missing | n/a | Not modeled. |
| `sk_category_name` | string | Missing | n/a | Not modeled. |
| `state` | string | Client only | `pkg/client/models.go` | Parsed but not exposed on the resource. |
| `subsidiary` | string array | Client only | `pkg/client/models.go` | Parsed but not exposed on the resource. |
| `tax_address` | object | Missing | n/a | Not modeled. |
| `total_spend_all_time` | money object | Client only | `pkg/client/models.go` | Parsed but not exposed on the resource. |
| `vendor_type` | enum | Client only | `pkg/client/models.go` | Parsed but not exposed on the resource. |

## Vendor agreement fields

Fields from `POST /developer/v1/vendors/agreements` and `GET /developer/v1/vendors/agreements/{agreement_id}`.

| Ramp field | Type / enum | Connector support | Code path | Notes |
|---|---|---|---|---|
| `id` | string | Full | `pkg/client/vm_models.go`, `pkg/connector/vendor_agreements.go` | Stable vendor agreement resource ID. |
| `name` | string | Full | `pkg/client/vm_models.go`, `pkg/connector/vendor_agreements.go` | Resource display name and VendorAgreementTrait agreement name. |
| `payee_id` / `payee.business_vendor.uuid` | string | Full | `pkg/client/vm_models.go`, `pkg/connector/vendor_agreements.go` | Emitted as VendorTrait `vendor_id`; single-fetch targeted sync prefers `payee.business_vendor.uuid` when present. |
| `payee_name` / `payee.name` | string | Full | `pkg/client/vm_models.go`, `pkg/connector/vendor_agreements.go` | Emitted as VendorTrait `vendor_name`. |
| `start_date` | date string | Full | `pkg/client/vm_models.go`, `pkg/connector/vendor_agreements.go` | Emitted as VendorAgreementTrait start date when parseable. |
| `end_date` | date string | Full | `pkg/client/vm_models.go`, `pkg/connector/vendor_agreements.go` | Emitted as VendorAgreementTrait end date when parseable. |
| `last_date_to_terminate` | date string | Full | `pkg/client/vm_models.go`, `pkg/connector/vendor_agreements.go` | Emitted when present and parseable. |
| `auto_renewal` | boolean | Full | `pkg/client/vm_models.go`, `pkg/connector/vendor_agreements.go` | Emitted as VendorAgreementTrait auto-renewal. |
| `renewal_status` | enum | Full | `pkg/client/vm_models.go`, `pkg/connector/vendor_agreements.go` | Normalized to VendorAgreementTrait renewal status and preserved as raw string. |
| `total_value` | money object | Full | `pkg/client/vm_models.go`, `pkg/connector/vendor_agreements.go` | Emitted as VendorAgreementTrait total value when currency is present. |
| `contract_owners` | array | Full | `pkg/client/vm_models.go`, `pkg/connector/vendor_agreements.go` | Synced as `contract_owner` grants to Ramp user resources. List results are cached to avoid per-agreement GETs during full sync. |
| `description` | string | Client only | `pkg/client/vm_models.go` | Parsed but not exposed on the resource. |
| `is_up_for_renewal` | boolean | Client only | `pkg/client/vm_models.go` | Parsed but not exposed on the resource. |
| `is_snoozed` | boolean | Client only | `pkg/client/vm_models.go` | Parsed from list but not exposed on the resource. |
| `notifications_on` | boolean | Client only | `pkg/client/vm_models.go` | Parsed but not exposed on the resource. |
| `currency` | string | Client only | `pkg/client/vm_models.go` | Parsed; `total_value.currency_code` is used for emitted money. |
| `logo` / `payee.image_url` | string | Client only | `pkg/client/vm_models.go` | Parsed but not exposed on the resource. |
| `days_remaining` | object | Client only | `pkg/client/vm_models.go` | Parsed but not exposed on the resource. |
| `line_items` | array | Client only | `pkg/client/vm_models.go`, `pkg/connector/vendor_agreements.go` | Single-fetch returns opaque line items; parsed as `[]map[string]any` but not emitted until the shape is verified. |
| `created_at`, `updated_at`, `deleted_at`, `archived_at` | date strings | Client only | `pkg/client/vm_models.go` | Parsed but not exposed on the resource. |

## Audit-log event support

`GET /developer/v1/audit-logs/events` is used only when `audit-log-events` / `BATON_AUDIT_LOG_EVENTS` is enabled.

| Ramp audit-log field | Connector support | Code path | Notes |
|---|---|---|---|
| `id` | Full | `pkg/client/audit_log.go`, `pkg/connector/audit_event_feed.go` | Event ID. |
| `event_time` | Full | `pkg/client/audit_log.go`, `pkg/connector/audit_event_feed.go` | Used as event timestamp and high-water mark. |
| `event_type` | Partial | `pkg/connector/audit_event_feed.go` | Only vendor-management event types that imply vendor/vendor-agreement resource changes are emitted. |
| `primary_reference.resource_name` | Partial | `pkg/connector/audit_event_feed.go` | Only `Vendor / Merchant` references are emitted. |
| `primary_reference.id` | Full for emitted events | `pkg/connector/audit_event_feed.go` | Used as the changed vendor or vendor-agreement resource ID. |
| `primary_reference.url` | Partial | `pkg/connector/audit_event_feed.go` | `/contracts/...` maps to `vendor_agreement`; other accepted vendor-management URLs map to `vendor`. |
| User or role audit events | Missing | n/a | Not emitted until Ramp audit references are verified to carry the same user IDs the connector syncs and targeted user sync is implemented. |

## OAuth scope support

| Connector behavior | Ramp scope |
|---|---|
| Read users and derive role grants | `users:read` |
| Read vendors and vendor agreements | `vendors:read` |
| Create/deactivate/reactivate users when provisioning is enabled | `users:write` |
| Grant/revoke vendor ownership when provisioning is enabled | `vendors:write` |
| Read audit-log events when `audit-log-events` is enabled | `audit_logs:read` |
| Business lookup | Not used; `business:read` is not required. |
| Vendor agreements | No separate scope; Ramp uses `vendors:read`. |

## Vendor update fields

Fields from `PATCH /developer/v1/vendors/{vendor_id}`.

| Ramp field | Type | Connector support | Code path | Notes |
|---|---|---|---|---|
| `vendor_owner_id` | string or null | Code-only / undocumented | `pkg/client/vendors.go`, `pkg/connector/vendors.go` | Existing connector code sends this field for entitlement grant/revoke, but the current public PATCH schema does not list it. |
| `accounting_vendor_remote_id` | string | Missing | n/a | Not managed. |
| `address` | object | Missing | n/a | Not managed. |
| `country` | string | Missing | n/a | Not managed. |
| `description` | string | Missing | n/a | Not managed. |
| `external_vendor_id` | string | Missing | n/a | Not managed. |
| `is_active` | boolean | Missing | n/a | Not managed. |
| `state` | string | Missing | n/a | Not managed. |
| `vendor_tracking_category_option_id` | string | Missing | n/a | Not managed. |
