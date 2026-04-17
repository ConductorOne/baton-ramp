package connector

import (
	"context"
	"fmt"

	config_sdk "github.com/conductorone/baton-sdk/pb/c1/config/v1"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/actions"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/structpb"
)

func (d *Connector) GlobalActions(ctx context.Context, registry actions.ActionRegistry) error {
	l := ctxzap.Extract(ctx)

	disableUserSchema := &v2.BatonActionSchema{
		Name:        "disable_user",
		DisplayName: "Disable User",
		Description: "Deactivate a Ramp user. The user will no longer be able to log in, spend on cards, or receive notifications.",
		Arguments: []*config_sdk.Field{
			{
				Name:        "user_id",
				DisplayName: "User ID",
				Description: "The Ramp user ID to deactivate",
				IsRequired:  true,
				Field: &config_sdk.Field_StringField{
					StringField: &config_sdk.StringField{},
				},
			},
		},
		ReturnTypes: []*config_sdk.Field{
			{
				Name:        "success",
				DisplayName: "Success",
				Field:       &config_sdk.Field_BoolField{},
			},
		},
		ActionType: []v2.ActionType{
			v2.ActionType_ACTION_TYPE_ACCOUNT,
			v2.ActionType_ACTION_TYPE_ACCOUNT_DISABLE,
		},
	}

	if err := registry.Register(ctx, disableUserSchema, func(ctx context.Context, args *structpb.Struct) (*structpb.Struct, annotations.Annotations, error) {
		return d.handleDisableUser(ctx, args)
	}); err != nil {
		l.Error("failed to register disable_user action", zap.Error(err))
		return err
	}

	enableUserSchema := &v2.BatonActionSchema{
		Name:        "enable_user",
		DisplayName: "Enable User",
		Description: "Reactivate a Ramp user. The user can log in to Ramp again, spend on their previously issued cards, and resume receiving notifications.",
		Arguments: []*config_sdk.Field{
			{
				Name:        "user_id",
				DisplayName: "User ID",
				Description: "The Ramp user ID to reactivate",
				IsRequired:  true,
				Field: &config_sdk.Field_StringField{
					StringField: &config_sdk.StringField{},
				},
			},
		},
		ReturnTypes: []*config_sdk.Field{
			{
				Name:        "success",
				DisplayName: "Success",
				Field:       &config_sdk.Field_BoolField{},
			},
		},
		ActionType: []v2.ActionType{
			v2.ActionType_ACTION_TYPE_ACCOUNT,
			v2.ActionType_ACTION_TYPE_ACCOUNT_ENABLE,
		},
	}

	if err := registry.Register(ctx, enableUserSchema, func(ctx context.Context, args *structpb.Struct) (*structpb.Struct, annotations.Annotations, error) {
		return d.handleEnableUser(ctx, args)
	}); err != nil {
		l.Error("failed to register enable_user action", zap.Error(err))
		return err
	}

	return nil
}

func (d *Connector) handleDisableUser(ctx context.Context, args *structpb.Struct) (*structpb.Struct, annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	userID, err := requireUserID(args)
	if err != nil {
		return nil, nil, err
	}

	l.Debug("disabling user", zap.String("user_id", userID))

	var annos annotations.Annotations
	ratelimitData, err := d.client.DeactivateUser(ctx, userID)
	annos.WithRateLimiting(ratelimitData)
	if err != nil {
		return &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"success": structpb.NewBoolValue(false),
			},
		}, annos, fmt.Errorf("ramp-connector: failed to disable user: %w", err)
	}

	return &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"success": structpb.NewBoolValue(true),
		},
	}, annos, nil
}

func (d *Connector) handleEnableUser(ctx context.Context, args *structpb.Struct) (*structpb.Struct, annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	userID, err := requireUserID(args)
	if err != nil {
		return nil, nil, err
	}

	l.Debug("enabling user", zap.String("user_id", userID))

	var annos annotations.Annotations
	ratelimitData, err := d.client.ReactivateUser(ctx, userID)
	annos.WithRateLimiting(ratelimitData)
	if err != nil {
		return &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"success": structpb.NewBoolValue(false),
			},
		}, annos, fmt.Errorf("ramp-connector: failed to enable user: %w", err)
	}

	return &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"success": structpb.NewBoolValue(true),
		},
	}, annos, nil
}

func requireUserID(args *structpb.Struct) (string, error) {
	userIDValue, ok := args.Fields["user_id"]
	if !ok {
		return "", fmt.Errorf("ramp-connector: user_id parameter is required")
	}
	userID := userIDValue.GetStringValue()
	if userID == "" {
		return "", fmt.Errorf("ramp-connector: user_id cannot be empty")
	}
	return userID, nil
}
