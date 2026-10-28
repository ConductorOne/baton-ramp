package connector

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/conductorone/baton-ramp/pkg/client"
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
	// resolve turns the non-empty profile value into a Ramp ID.
	resolve func(ctx context.Context, cli *client.Client, value string) (string, error)
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
// fields to Ramp IDs before any user is created. Returns (nil, nil) when none
// of the fields are set — attribute-less creation makes ZERO lookup calls.
// attrsEnabled reflects the provisioning-attributes config flag; when off, any
// set attribute is an error rather than a silent drop.
func resolveCreationAttributes(ctx context.Context, cli *client.Client, fields map[string]*structpb.Value, attrsEnabled bool) (*optionalAttributes, error) {
	set := make(map[string]string)
	for _, entry := range creationAttributeTable {
		val := fields[entry.profileKey]
		if val == nil || val.GetStringValue() == "" {
			continue
		}
		if !attrsEnabled {
			return nil, unresolvedAttrError(entry.profileKey, val.GetStringValue(),
				"the provisioning-attributes config flag is off; enable it (adds the departments:read and locations:read OAuth scopes — grant them to the Ramp OAuth app first) or clear the field")
		}
		set[entry.profileKey] = val.GetStringValue()
	}
	if len(set) == 0 {
		return nil, nil
	}

	attrs := &optionalAttributes{}
	for _, entry := range creationAttributeTable {
		value, ok := set[entry.profileKey]
		if !ok {
			continue
		}
		resolved, err := entry.resolve(ctx, cli, value)
		if err != nil {
			return nil, err
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
	return attrs, nil
}

// resolveDirectManager maps an email to a Ramp user ID via FindUserByEmail.
func resolveDirectManager(ctx context.Context, cli *client.Client, email string) (string, error) {
	user, _, err := cli.FindUserByEmail(ctx, email)
	if err != nil {
		// Upstream lookup errors propagate wrapped with their own code.
		return "", fmt.Errorf("ramp-connector: cannot set %s to %q: error looking up Ramp user: %w", profileKeyDirectManagerEmail, email, err)
	}
	if user == nil {
		return "", unresolvedAttrError(profileKeyDirectManagerEmail, email,
			"no Ramp user with that email; grant the user a Ramp account or use an existing email")
	}
	return user.ID, nil
}

// resolveIDOrName resolves a value that is either a Ramp ID or a name: probe
// GET /{resource}/{id} first; NotFound or InvalidArgument means "not an ID"
// and falls through to a bounded case-insensitive name walk. Any other error
// code propagates wrapped — never rewritten to "no match".
func resolveIDOrName(ctx context.Context, cli *client.Client, attr, value string, probeID func(ctx context.Context, id string) (string, bool, error), walkNames func(ctx context.Context, name string) ([]string, error)) (string, error) {
	// Fast path: case-insensitive exact match against a locally known value is
	// not possible without a cache (banned), so probe the ID endpoint first.
	id, isID, err := probeID(ctx, value)
	if err == nil && isID {
		return id, nil
	}
	if err != nil {
		if code := grpcstatus.Code(err); code != codes.NotFound && code != codes.InvalidArgument {
			// Upstream lookup errors (403/5xx/429) propagate wrapped with
			// their own code — never rewritten to "no match".
			return "", fmt.Errorf("ramp-connector: cannot set %s to %q: error looking up Ramp %s: %w", attr, value, attr, err)
		}
	}

	matches, err := walkNames(ctx, value)
	if err != nil {
		var uaErr *unresolvedAttrErrorT
		if errors.As(err, &uaErr) {
			// Terminal fail-closed result (e.g. page cap exhausted) — surface as-is.
			return "", err
		}
		if code := grpcstatus.Code(err); code != codes.NotFound && code != codes.InvalidArgument {
			return "", fmt.Errorf("ramp-connector: cannot set %s to %q: error searching Ramp %s: %w", attr, value, attr, err)
		}
		return "", unresolvedAttrError(attr, value, fmt.Sprintf("error searching Ramp %ss; verify the value and grant the required OAuth scopes", attr))
	}
	switch len(matches) {
	case 0:
		return "", unresolvedAttrError(attr, value, fmt.Sprintf("no Ramp %s with that ID or name; use a Ramp ID or the exact name of an existing %s", attr, attr))
	case 1:
		return matches[0], nil
	default:
		return "", unresolvedAttrError(attr, value, fmt.Sprintf("multiple Ramp %ss match that name (IDs: %s); use a Ramp ID instead", attr, strings.Join(matches, ", ")))
	}
}

// resolveDepartment resolves the department profile field to a Ramp department ID.
func resolveDepartment(ctx context.Context, cli *client.Client, value string) (string, error) {
	return resolveIDOrName(ctx, cli, profileKeyDepartment, value,
		func(ctx context.Context, id string) (string, bool, error) {
			department, _, err := cli.GetDepartment(ctx, id)
			if err != nil {
				return "", false, err
			}
			return department.ID, true, nil
		},
		func(ctx context.Context, name string) ([]string, error) {
			var matches []string
			pagination := ""
			for page := 1; ; page++ {
				if page > maxAttrLookupPages {
					return nil, unresolvedAttrError(profileKeyDepartment, name,
						fmt.Sprintf("list exhausted after %d pages", maxAttrLookupPages))
				}
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				resp, _, err := cli.ListDepartments(ctx, pagination)
				if err != nil {
					return nil, err
				}
				for _, department := range resp.Departments {
					if department != nil && strings.EqualFold(department.Name, name) {
						matches = append(matches, department.ID)
					}
				}
				if resp.Pagination == "" {
					return matches, nil
				}
				pagination = resp.Pagination
			}
		})
}

// resolveLocation resolves the location profile field to a Ramp location ID.
func resolveLocation(ctx context.Context, cli *client.Client, value string) (string, error) {
	return resolveIDOrName(ctx, cli, profileKeyLocation, value,
		func(ctx context.Context, id string) (string, bool, error) {
			location, _, err := cli.GetLocation(ctx, id)
			if err != nil {
				return "", false, err
			}
			return location.ID, true, nil
		},
		func(ctx context.Context, name string) ([]string, error) {
			var matches []string
			pagination := ""
			for page := 1; ; page++ {
				if page > maxAttrLookupPages {
					return nil, unresolvedAttrError(profileKeyLocation, name,
						fmt.Sprintf("list exhausted after %d pages", maxAttrLookupPages))
				}
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				resp, _, err := cli.ListLocations(ctx, pagination)
				if err != nil {
					return nil, err
				}
				for _, location := range resp.Locations {
					if location != nil && strings.EqualFold(location.Name, name) {
						matches = append(matches, location.ID)
					}
				}
				if resp.Pagination == "" {
					return matches, nil
				}
				pagination = resp.Pagination
			}
		})
}
