package connector

import (
	"testing"

	"github.com/conductorone/baton-ramp/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	resourceSdk "github.com/conductorone/baton-sdk/pkg/types/resource"
)

func TestUserResourceStatus(t *testing.T) {
	tests := []struct {
		rampStatus string
		want       v2.Status_ResourceStatus
	}{
		{rampStatus: "USER_ACTIVE", want: v2.Status_RESOURCE_STATUS_ENABLED},
		{rampStatus: "USER_ONBOARDING", want: v2.Status_RESOURCE_STATUS_ENABLED},
		{rampStatus: "USER_INACTIVE", want: v2.Status_RESOURCE_STATUS_DISABLED},
		{rampStatus: "USER_SUSPENDED", want: v2.Status_RESOURCE_STATUS_DISABLED},
	}
	for _, tt := range tests {
		t.Run(tt.rampStatus, func(t *testing.T) {
			r, err := userResource(&client.User{
				ID:        "u1",
				Email:     "jane.doe@example.com",
				FirstName: "Jane",
				LastName:  "Doe",
				Status:    tt.rampStatus,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := r.GetStatus().GetStatus(); got != tt.want {
				t.Fatalf("resource status = %v, want %v", got, tt.want)
			}
			if got := resourceSdk.GetStatus(r).GetStatus(); got != tt.want {
				t.Fatalf("resolved status = %v, want %v", got, tt.want)
			}
		})
	}
}
