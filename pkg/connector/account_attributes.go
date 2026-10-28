package connector

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/conductorone/baton-ramp/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// maxAttrLookupPages caps the name-walk pagination so a misbehaving token
// chain cannot spin forever. Exceeding it fails creation closed.
const maxAttrLookupPages = 50

// Profile keys that carry account-creation attributes on the AccountInfo
// profile. They map onto POST /developer/v1/users/deferred fields
// direct_manager_id, department_id and location_id.
const (
	profileKeyDirectManagerEmail = "direct_manager_email"
	profileKeyDepartment         = "department"
	profileKeyLocation           = "location"
)

// optionalAttributes holds the resolved Ramp IDs for the optional
// account-creation attributes. Empty string means the attribute was not set.
type optionalAttributes struct {
	DepartmentID    string
	DirectManagerID string
	LocationID      string
}

// unresolvedAttrErrorT is the typed form of the fail-closed InvalidArgument
// error so callers can distinguish it from upstream lookup failures.
type unresolvedAttrErrorT struct {
	msg string
}

func (e *unresolvedAttrErrorT) Error() string { return e.msg }

func (e *unresolvedAttrErrorT) GRPCStatus() *grpcstatus.Status {
	return grpcstatus.New(codes.InvalidArgument, e.msg)
}

// unresolvedAttrError builds the fail-closed InvalidArgument error for a
// value that could not be resolved to a Ramp ID. It names the profile field,
// the supplied value, what was tried, and what to do.
func unresolvedAttrError(attr, value, reason string) error {
	return &unresolvedAttrErrorT{msg: fmt.Sprintf(
		"ramp-connector: cannot set %s to %q: %s", attr, value, reason)}
}

// attrEntry describes one resolvable account-creation attribute.
type attrEntry struct {
	// profileKey is the AccountInfo profile field carrying the user-supplied value.
	profileKey string
	// resolve turns the non-empty profile value into a Ramp ID. It threads the
	// last rate-limit data seen so the SDK can back off when Ramp is throttling.
	resolve func(ctx context.Context, cli *client.Client, value string) (string, *v2.RateLimitDescription, error)
}

// creationAttributeTable is the declarative resolver table. Order matters
// only for error reporting; each entry resolves independently.
var creationAttributeTable = []attrEntry{
	{
		profileKey: profileKeyDirectManagerEmail,
		resolve:    resolveDirectManager,
	},
	{
		profileKey: profileKeyDepartment,
		resolve:    resolveDepartment,
	},
	{
		profileKey: profileKeyLocation,
		resolve:    resolveLocation,
	},
}

// resolveCreationAttributes resolves the optional account-creation profile
// fields to Ramp IDs before any user is created. Returns (nil, nil, nil) when
// none of the fields are set — attribute-less creation makes ZERO lookup
// calls. attrsEnabled reflects the provisioning-attributes config flag; when
// off, any set attribute is an error rather than a silent drop. The returned
// rate-limit data is the last non-nil one seen during resolution, so a 429 on
// a lookup still reaches the SDK's annotations.
func resolveCreationAttributes(ctx context.Context, cli *client.Client, fields map[string]*structpb.Value, attrsEnabled bool) (*optionalAttributes, *v2.RateLimitDescription, error) {
	set := make(map[string]string)
	for _, entry := range creationAttributeTable {
		val := fields[entry.profileKey]
		if val == nil {
			continue
		}
		// Kind check BEFORE the flag-off guard: a present-but-non-string value
		// must fail closed, not silently bypass the guard. NullValue means the
		// field was explicitly unset (JSON null); empty strings are absent.
		switch kind := val.GetKind().(type) {
		case *structpb.Value_StringValue:
			if kind.StringValue == "" {
				continue
			}
			if !attrsEnabled {
				return nil, nil, unresolvedAttrError(entry.profileKey, kind.StringValue,
					"the provisioning-attributes config flag is off; enable it (adds the departments:read and locations:read OAuth scopes — grant them to the Ramp OAuth app first) or clear the field")
			}
			set[entry.profileKey] = kind.StringValue
		case *structpb.Value_NullValue:
			continue
		default:
			return nil, nil, unresolvedAttrError(entry.profileKey, val.String(),
				"profile field must be a string value")
		}
	}
	if len(set) == 0 {
		return nil, nil, nil
	}

	attrs := &optionalAttributes{}
	var lastRL *v2.RateLimitDescription
	for _, entry := range creationAttributeTable {
		value, ok := set[entry.profileKey]
		if !ok {
			continue
		}
		resolved, rl, err := entry.resolve(ctx, cli, value)
		if rl != nil {
			lastRL = rl
		}
		if err != nil {
			return nil, lastRL, err
		}
		// Belt-and-braces: an empty resolved ID must never be silently dropped
		// from the create body (fail-closed, AC3).
		if resolved == "" {
			return nil, lastRL, unresolvedAttrError(entry.profileKey, value, "resolved to an empty Ramp ID")
		}
		switch entry.profileKey {
		case profileKeyDirectManagerEmail:
			attrs.DirectManagerID = resolved
		case profileKeyDepartment:
			attrs.DepartmentID = resolved
		case profileKeyLocation:
			attrs.LocationID = resolved
		}
	}
	return attrs, lastRL, nil
}

// resolveDirectManager maps an email to a Ramp user ID via FindUserByEmail.
func resolveDirectManager(ctx context.Context, cli *client.Client, email string) (string, *v2.RateLimitDescription, error) {
	user, rl, err := cli.FindUserByEmail(ctx, email)
	if err != nil {
		// The identical NotFound means "no user" on CreateAccount's own
		// short-circuit path (users.go), so map it to the fail-closed error
		// instead of an upstream failure. All other codes propagate wrapped.
		if grpcstatus.Code(err) == codes.NotFound {
			return "", rl, unresolvedAttrError(profileKeyDirectManagerEmail, email,
				"no Ramp user with that email; grant the user a Ramp account or use an existing email")
		}
		return "", rl, fmt.Errorf("ramp-connector: cannot set %s to %q: error looking up Ramp user: %w", profileKeyDirectManagerEmail, email, err)
	}
	if user == nil {
		return "", rl, unresolvedAttrError(profileKeyDirectManagerEmail, email,
			"no Ramp user with that email; grant the user a Ramp account or use an existing email")
	}
	return user.ID, rl, nil
}

// resolveIDOrName resolves a value that is either a Ramp ID or a name: probe
// GET /{resource}/{id} first; NotFound or InvalidArgument means "not an ID"
// and falls through to a bounded case-insensitive name walk. Any other error
// code propagates wrapped — never rewritten to "no match". Rate-limit data
// from both hops is threaded through.
func resolveIDOrName(
	ctx context.Context,
	cli *client.Client,
	attr, value string,
	probeID func(ctx context.Context, id string) (string, bool, *v2.RateLimitDescription, error),
	walkNames func(ctx context.Context, name string) ([]string, *v2.RateLimitDescription, error),
) (string, *v2.RateLimitDescription, error) {
	// Fast path: case-insensitive exact match against a locally known value is
	// not possible without a cache (banned), so probe the ID endpoint first.
	id, isID, rl, err := probeID(ctx, value)
	if err == nil && isID {
		return id, rl, nil
	}
	if err != nil {
		if code := grpcstatus.Code(err); code != codes.NotFound && code != codes.InvalidArgument {
			// Upstream lookup errors (403/5xx/429) propagate wrapped with
			// their own code — never rewritten to "no match".
			return "", rl, fmt.Errorf("ramp-connector: cannot set %s to %q: error looking up Ramp %s: %w", attr, value, attr, err)
		}
	}

	matches, walkRL, err := walkNames(ctx, value)
	if walkRL != nil {
		rl = walkRL
	}
	if err != nil {
		var uaErr *unresolvedAttrErrorT
		if errors.As(err, &uaErr) {
			// Terminal fail-closed result (e.g. page cap exhausted) — surface as-is.
			return "", rl, err
		}
		if code := grpcstatus.Code(err); code != codes.NotFound && code != codes.InvalidArgument {
			return "", rl, fmt.Errorf("ramp-connector: cannot set %s to %q: error searching Ramp %s: %w", attr, value, attr, err)
		}
		return "", rl, unresolvedAttrError(attr, value, fmt.Sprintf("Ramp %ss search failed; verify the value and the provisioning-attributes scopes", attr))
	}
	switch len(matches) {
	case 0:
		return "", rl, unresolvedAttrError(attr, value, fmt.Sprintf("no Ramp %s with that ID or name; use a Ramp ID or the exact name of an existing %s", attr, attr))
	case 1:
		return matches[0], rl, nil
	default:
		return "", rl, unresolvedAttrError(attr, value, fmt.Sprintf("multiple Ramp %ss match that name (IDs: %s); use a Ramp ID instead", attr, strings.Join(matches, ", ")))
	}
}

// walkIDsByName is the shared bounded name walk: one page per loop iteration,
// ctx checked, capped at maxAttrLookupPages (terminal fail-closed error), with
// rate-limit data threaded from every page. listOne adapts one client list
// call to (matched IDs, next token, rate-limit data, error).
func walkIDsByName(
	ctx context.Context,
	attr, value string,
	listOne func(ctx context.Context, pagination string) ([]string, string, *v2.RateLimitDescription, error),
) ([]string, *v2.RateLimitDescription, error) {
	var matches []string
	var lastRL *v2.RateLimitDescription
	pagination := ""
	for page := 1; ; page++ {
		if page > maxAttrLookupPages {
			return nil, lastRL, unresolvedAttrError(attr, value,
				fmt.Sprintf("list exhausted after %d pages", maxAttrLookupPages))
		}
		if ctx.Err() != nil {
			return nil, lastRL, ctx.Err()
		}
		pageMatches, next, rl, err := listOne(ctx, pagination)
		if rl != nil {
			lastRL = rl
		}
		if err != nil {
			return nil, lastRL, err
		}
		matches = append(matches, pageMatches...)
		if next == "" {
			return matches, lastRL, nil
		}
		pagination = next
	}
}

// resolveDepartment resolves the department profile field to a Ramp department ID.
func resolveDepartment(ctx context.Context, cli *client.Client, value string) (string, *v2.RateLimitDescription, error) {
	return resolveIDOrName(ctx, cli, profileKeyDepartment, value,
		func(ctx context.Context, id string) (string, bool, *v2.RateLimitDescription, error) {
			department, rl, err := cli.GetDepartment(ctx, id)
			if err != nil {
				return "", false, rl, err
			}
			// An empty decoded ID is not a match: fall through to the name walk.
			if department == nil || department.ID == "" {
				return "", false, rl, grpcstatus.Errorf(codes.NotFound, "empty id")
			}
			return department.ID, true, rl, nil
		},
		func(ctx context.Context, name string) ([]string, *v2.RateLimitDescription, error) {
			return walkIDsByName(ctx, profileKeyDepartment, name, func(ctx context.Context, pagination string) ([]string, string, *v2.RateLimitDescription, error) {
				resp, rl, err := cli.ListDepartments(ctx, pagination)
				if err != nil {
					return nil, "", rl, err
				}
				var ids []string
				for _, department := range resp.Departments {
					if department != nil && strings.EqualFold(department.Name, name) {
						ids = append(ids, department.ID)
					}
				}
				return ids, resp.Pagination, rl, nil
			})
		})
}

// resolveLocation resolves the location profile field to a Ramp location ID.
func resolveLocation(ctx context.Context, cli *client.Client, value string) (string, *v2.RateLimitDescription, error) {
	return resolveIDOrName(ctx, cli, profileKeyLocation, value,
		func(ctx context.Context, id string) (string, bool, *v2.RateLimitDescription, error) {
			location, rl, err := cli.GetLocation(ctx, id)
			if err != nil {
				return "", false, rl, err
			}
			// An empty decoded ID is not a match: fall through to the name walk.
			if location == nil || location.ID == "" {
				return "", false, rl, grpcstatus.Errorf(codes.NotFound, "empty id")
			}
			return location.ID, true, rl, nil
		},
		func(ctx context.Context, name string) ([]string, *v2.RateLimitDescription, error) {
			return walkIDsByName(ctx, profileKeyLocation, name, func(ctx context.Context, pagination string) ([]string, string, *v2.RateLimitDescription, error) {
				resp, rl, err := cli.ListLocations(ctx, pagination)
				if err != nil {
					return nil, "", rl, err
				}
				var ids []string
				for _, location := range resp.Locations {
					if location != nil && strings.EqualFold(location.Name, name) {
						ids = append(ids, location.ID)
					}
				}
				return ids, resp.Pagination, rl, nil
			})
		})
}
