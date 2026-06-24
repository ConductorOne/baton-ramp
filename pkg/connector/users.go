package connector

import (
	"context"
	"fmt"
	"sort"

	"github.com/conductorone/baton-ramp/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	resourceSdk "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// validCreateRoles is the set of roles the Ramp API accepts when creating a user via the deferred endpoint.
// The read enum includes additional UNBUNDLED_* roles that are not accepted by create or update.
var validCreateRoles = map[string]bool{
	roleIDAuditor:            true,
	roleIDBusinessAdmin:      true,
	roleIDBusinessBookkeeper: true,
	roleIDBusinessUser:       true,
	roleIDGuestUser:          true,
	roleIDITAdmin:            true,
}

type userBuilder struct {
	client *client.Client
}

func (o *userBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return userResourceType
}

func userResource(u *client.User) (*v2.Resource, error) {
	status := v2.UserTrait_Status_STATUS_ENABLED
	if u.Status != "USER_ACTIVE" && u.Status != "USER_ONBOARDING" {
		status = v2.UserTrait_Status_STATUS_DISABLED
	}

	return resourceSdk.NewResource(
		fmt.Sprintf("%s %s", u.FirstName, u.LastName),
		userResourceType,
		u.ID,
		resourceSdk.WithUserTrait(
			resourceSdk.WithEmail(u.Email, true),
			resourceSdk.WithStatus(status),
			resourceSdk.WithUserLogin(u.Email),
		),
	)
}

// List returns all the users from the database as resource objects.
// Users include a UserTrait because they are the 'shape' of a standard user.
func (o *userBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, pToken *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	var annos annotations.Annotations

	usersResponse, ratelimitData, err := o.client.ListUsers(ctx, pToken.Token)
	if err != nil {
		return nil, "", nil, err
	}
	annos.WithRateLimiting(ratelimitData)

	rv := make([]*v2.Resource, 0, len(usersResponse.Users))
	unknownRoles := map[string]struct{}{}
	unknownRoleUserCount := 0
	for _, u := range usersResponse.Users {
		if !isKnownRampRole(u.Role) {
			unknownRoleUserCount++
			unknownRoles[normalizeRampRole(u.Role)] = struct{}{}
		}
		resource, err := userResource(u)
		if err != nil {
			return nil, "", annos, fmt.Errorf("baton-ramp: failed to create resource for user %s: %w", u.ID, err)
		}
		rv = append(rv, resource)
	}
	if len(unknownRoles) > 0 {
		roles := make([]string, 0, len(unknownRoles))
		for role := range unknownRoles {
			roles = append(roles, role)
		}
		sort.Strings(roles)
		ctxzap.Extract(ctx).Warn(
			"ramp-connector: users have unknown roles",
			zap.Strings("roles", roles),
			zap.Int("user_count", unknownRoleUserCount),
		)
	}

	return rv, usersResponse.Pagination, annos, nil
}

// Entitlements always returns an empty slice for users.
func (o *userBuilder) Entitlements(_ context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

// Grants always returns an empty slice for users since they don't have any entitlements.
func (o *userBuilder) Grants(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

func (o *userBuilder) CreateAccountCapabilityDetails(ctx context.Context) (*v2.CredentialDetailsAccountProvisioning, annotations.Annotations, error) {
	return &v2.CredentialDetailsAccountProvisioning{
		SupportedCredentialOptions: []v2.CapabilityDetailCredentialOption{
			v2.CapabilityDetailCredentialOption_CAPABILITY_DETAIL_CREDENTIAL_OPTION_NO_PASSWORD,
		},
		PreferredCredentialOption: v2.CapabilityDetailCredentialOption_CAPABILITY_DETAIL_CREDENTIAL_OPTION_NO_PASSWORD,
	}, nil, nil
}

func (o *userBuilder) CreateAccount(
	ctx context.Context,
	accountInfo *v2.AccountInfo,
	_ *v2.LocalCredentialOptions,
) (connectorbuilder.CreateAccountResponse, []*v2.PlaintextData, annotations.Annotations, error) {
	var annos annotations.Annotations

	profileFields := accountInfo.Profile.GetFields()

	emailVal := profileFields["email"]
	if emailVal == nil || emailVal.GetStringValue() == "" {
		return nil, nil, nil, grpcstatus.Error(codes.InvalidArgument, "ramp-connector: email is required for account creation")
	}
	email := emailVal.GetStringValue()

	firstNameVal := profileFields["first_name"]
	if firstNameVal == nil || firstNameVal.GetStringValue() == "" {
		return nil, nil, nil, grpcstatus.Error(codes.InvalidArgument, "ramp-connector: first_name is required for account creation")
	}
	firstName := firstNameVal.GetStringValue()

	lastNameVal := profileFields["last_name"]
	if lastNameVal == nil || lastNameVal.GetStringValue() == "" {
		return nil, nil, nil, grpcstatus.Error(codes.InvalidArgument, "ramp-connector: last_name is required for account creation")
	}
	lastName := lastNameVal.GetStringValue()

	roleVal := profileFields["role"]
	if roleVal == nil || roleVal.GetStringValue() == "" {
		return nil, nil, nil, grpcstatus.Error(codes.InvalidArgument, "ramp-connector: role is required for account creation")
	}
	role := roleVal.GetStringValue()
	if !validCreateRoles[role] {
		return nil, nil, nil, grpcstatus.Errorf(codes.InvalidArgument,
			"ramp-connector: invalid role %q, must be one of AUDITOR, BUSINESS_ADMIN, BUSINESS_BOOKKEEPER, BUSINESS_USER, GUEST_USER, IT_ADMIN",
			role)
	}

	req := &client.CreateUserRequest{
		Email:     email,
		FirstName: firstName,
		LastName:  lastName,
		Role:      role,
	}

	task, ratelimitData, err := o.client.CreateUser(ctx, req)
	annos.WithRateLimiting(ratelimitData)
	if err != nil {
		return nil, nil, annos, fmt.Errorf("ramp-connector: failed to create user: %w", err)
	}

	ctxzap.Extract(ctx).Debug("ramp-connector: user invite sent, sync required to retrieve account", zap.String("task_id", task.ID))
	return &v2.CreateAccountResponse_ActionRequiredResult{
		Message:               fmt.Sprintf("User invite sent (task %s). Please sync after the user accepts the invite to retrieve their account.", task.ID),
		IsCreateAccountResult: true,
	}, nil, annos, nil
}

func newUserBuilder(client *client.Client) *userBuilder {
	return &userBuilder{
		client: client,
	}
}
