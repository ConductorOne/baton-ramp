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
| `GET /developer/v1/vendors` | Yes | Partial | Lists vendor resources using `id` and `name`; keeps `vendor_owner_id` in the client model for owner grants. |
| `GET /developer/v1/vendors/{vendor_id}` | Yes | Partial | Fetches a vendor to read `vendor_owner_id` for grants and idempotency. |
| `PATCH /developer/v1/vendors/{vendor_id}` | Yes | Partial / undocumented | Existing connector code sends `vendor_owner_id` to set or clear owner, but the current public PATCH schema does not list `vendor_owner_id`. Other mutable vendor fields are not managed. |
| Other vendor endpoints | No | Missing | Vendor agreements, contacts, bank accounts, credits, children, and document flows are out of scope. |

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
| `BUSINESS_OWNER` | Owner | Yes | Yes | Yes |
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
| `role` | enum | Full | `pkg/connector/connector.go`, `pkg/connector/users.go` | Required; accepts the Ramp create enum. |
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
| `is_active` | boolean | Client only | `pkg/client/models.go` | Parsed but not exposed on the resource. |
| `code` | string | Client only | `pkg/client/models.go` | Legacy/simple model field; current Ramp vendor docs use richer identifiers. |
| `accounting_vendor_remote_id` | string | Missing | n/a | Not modeled. |
| `address` | object | Missing | n/a | Not modeled. |
| `addresses` | array | Missing | n/a | Not modeled. |
| `billing_frequency` | enum | Missing | n/a | Not modeled. |
| `contacts` | array | Missing | n/a | Not modeled. |
| `country` | string | Missing | n/a | Not modeled. |
| `created_at` | date string | Missing | n/a | Not modeled. |
| `custom_form_collection_response` | object | Missing | n/a | Returned by fetch detail; not modeled. |
| `custom_record_fields` | array | Missing | n/a | Returned by fetch detail; not modeled. |
| `default_entity_id` | string | Missing | n/a | Not modeled. |
| `default_payment_method` | object | Missing | n/a | Not modeled. |
| `description` | string | Missing | n/a | Not modeled. |
| `external_vendor_id` | string | Missing | n/a | Not modeled. |
| `federal_tax_classification` | enum | Missing | n/a | Not modeled. |
| `is_deletable` | boolean | Missing | n/a | Not modeled. |
| `merchant_id` | string | Missing | n/a | Not modeled. |
| `name_legal` | string | Missing | n/a | Not modeled. |
| `parent_vendor_id` | string | Missing | n/a | Not modeled. |
| `sk_category_id` | number | Missing | n/a | Not modeled. |
| `sk_category_name` | string | Missing | n/a | Not modeled. |
| `state` | string | Missing | n/a | Not modeled. |
| `subsidiary` | string array | Missing | n/a | Not modeled. |
| `tax_address` | object | Missing | n/a | Not modeled. |
| `total_spend_all_time` | money object | Missing | n/a | Not modeled. |
| `total_spend_last_30_days` | money object | Missing | n/a | Not modeled. |
| `total_spend_last_365_days` | money object | Missing | n/a | Not modeled. |
| `total_spend_ytd` | money object | Missing | n/a | Not modeled. |
| `vendor_type` | enum | Missing | n/a | Not modeled. |

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
