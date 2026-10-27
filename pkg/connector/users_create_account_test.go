package connector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/conductorone/baton-ramp/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"google.golang.org/protobuf/types/known/structpb"
)

type lookupResponse struct {
	status int
	body   string
}

var (
	noUsersLookup       = lookupResponse{status: http.StatusOK, body: `{"data":[],"page":{"next":""}}`}
	existingUsersLookup = lookupResponse{status: http.StatusOK, body: `{
		"data": [{
			"id": "user-1",
			"email": "Jane@Example.com",
			"first_name": "Jane",
			"last_name": "Doe",
			"role": "BUSINESS_USER",
			"status": "INVITE_PENDING"
		}],
		"page": {"next": ""}
	}`}
	notFoundLookup  = lookupResponse{status: http.StatusNotFound}
	forbiddenLookup = lookupResponse{status: http.StatusForbidden}
)

type createAccountTestServer struct {
	lookupResponses []lookupResponse
	lookupCalls     int
	createCalls     int
}

func newCreateAccountTestBuilder(t *testing.T, lookupResponses ...lookupResponse) (*userBuilder, *createAccountTestServer) {
	t.Helper()
	ts := &createAccountTestServer{lookupResponses: lookupResponses}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /developer/v1/users":
			if got := r.URL.Query().Get("email"); got != "jane@example.com" {
				t.Errorf("email query = %q, want jane@example.com", got)
			}
			if ts.lookupCalls >= len(ts.lookupResponses) {
				t.Fatalf("unexpected email lookup %d", ts.lookupCalls+1)
			}
			resp := ts.lookupResponses[ts.lookupCalls]
			ts.lookupCalls++
			w.WriteHeader(resp.status)
			_, _ = w.Write([]byte(resp.body))
		case "POST /developer/v1/users/deferred":
			ts.createCalls++
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"task-1"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	c, err := client.New(context.Background(), client.Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return newUserBuilder(c), ts
}

func createAccountTestInfo(t *testing.T) *v2.AccountInfo {
	t.Helper()
	profile, err := structpb.NewStruct(map[string]any{
		"email":      "jane@example.com",
		"first_name": "Jane",
		"last_name":  "Doe",
		"role":       "BUSINESS_USER",
	})
	if err != nil {
		t.Fatal(err)
	}
	return &v2.AccountInfo{Profile: profile}
}

func TestCreateAccountReturnsExistingUserWithoutCreating(t *testing.T) {
	builder, ts := newCreateAccountTestBuilder(t, existingUsersLookup)

	resp, _, _, err := builder.CreateAccount(context.Background(), createAccountTestInfo(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	result, ok := resp.(*v2.CreateAccountResponse_AlreadyExistsResult)
	if !ok {
		t.Fatalf("expected AlreadyExistsResult, got %T", resp)
	}
	if got := result.GetResource().GetId().GetResource(); got != "user-1" {
		t.Fatalf("resource id = %q, want user-1", got)
	}
	if !result.GetIsCreateAccountResult() {
		t.Fatal("expected IsCreateAccountResult to be true")
	}
	if ts.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", ts.createCalls)
	}
}

func TestCreateAccountCreatesUserAndRetryFindsIt(t *testing.T) {
	builder, ts := newCreateAccountTestBuilder(t, noUsersLookup, existingUsersLookup)

	resp, _, _, err := builder.CreateAccount(context.Background(), createAccountTestInfo(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := resp.(*v2.CreateAccountResponse_ActionRequiredResult); !ok {
		t.Fatalf("expected ActionRequiredResult, got %T", resp)
	}

	resp, _, _, err = builder.CreateAccount(context.Background(), createAccountTestInfo(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := resp.(*v2.CreateAccountResponse_AlreadyExistsResult); !ok {
		t.Fatalf("expected AlreadyExistsResult on retry, got %T", resp)
	}
	if ts.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", ts.createCalls)
	}
}

func TestCreateAccountCreatesUserWhenLookupReturnsNotFound(t *testing.T) {
	builder, ts := newCreateAccountTestBuilder(t, notFoundLookup)

	resp, _, _, err := builder.CreateAccount(context.Background(), createAccountTestInfo(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := resp.(*v2.CreateAccountResponse_ActionRequiredResult); !ok {
		t.Fatalf("expected ActionRequiredResult, got %T", resp)
	}
	if ts.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", ts.createCalls)
	}
}

func TestCreateAccountFailsWithoutCreatingWhenLookupFails(t *testing.T) {
	builder, ts := newCreateAccountTestBuilder(t, forbiddenLookup)

	if _, _, _, err := builder.CreateAccount(context.Background(), createAccountTestInfo(t), nil); err == nil {
		t.Fatal("expected error when user lookup fails")
	}
	if ts.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", ts.createCalls)
	}
}
