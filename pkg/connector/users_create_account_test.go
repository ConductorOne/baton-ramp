package connector

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/conductorone/baton-ramp/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
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
	return newUserBuilder(c, false), ts
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

// --- Account-creation attribute tests (provisioning-attributes flag on) ---

// attrLookupResponse is a canned response for one attribute-lookup route.
type attrLookupResponse struct {
	status int
	body   string
}

var (
	attrDeptByID    = attrLookupResponse{status: http.StatusOK, body: `{"id":"dept-1","name":"Engineering"}`}
	attrDeptByID404 = attrLookupResponse{status: http.StatusNotFound, body: `{"message":"not found"}`}
	attrDeptByID403 = attrLookupResponse{status: http.StatusForbidden, body: `{"message":"forbidden"}`}
	attrDeptListOne = attrLookupResponse{status: http.StatusOK, body: `{"data":[{"id":"dept-9","name":"Engineering"}],"page":{"next":""}}`}
	attrDeptListTwo = attrLookupResponse{status: http.StatusOK, body: `{"data":[{"id":"dept-1","name":"Engineering"},{"id":"dept-2","name":"Engineering"}],"page":{"next":""}}`}
	attrLocByID     = attrLookupResponse{status: http.StatusOK, body: `{"id":"loc-1","name":"HQ","entity_id":"ent-1"}`}
	attrLocByID404  = attrLookupResponse{status: http.StatusNotFound, body: `{"message":"not found"}`}
	attrLocListOne  = attrLookupResponse{status: http.StatusOK, body: `{"data":[{"id":"loc-9","name":"Remote"}],"page":{"next":""}}`}
)

// attributeTestServer serves the email lookup, deferred create and the
// department/location routes, recording create POST bodies.
type attributeTestServer struct {
	lookupResponse lookupResponse // email lookup (users?email=)
	deptByID       attrLookupResponse
	deptList       attrLookupResponse
	locByID        attrLookupResponse
	locList        attrLookupResponse

	// Optional X-RateLimit-Limit header values to prove annotation precedence.
	deptRateLimit   int
	createRateLimit int

	lookupCalls   int
	deptIDCalls   int
	deptListCalls int
	locIDCalls    int
	locListCalls  int
	createCalls   int
	createBodies  []string
}

func newAttributeTestBuilder(t *testing.T, ts *attributeTestServer) *userBuilder {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/developer/v1/users":
			switch got := r.URL.Query().Get("email"); got {
			case "jane@example.com", "manager@example.com", "ghost@example.com":
			default:
				t.Errorf("email query = %q, want jane@example.com", got)
			}
			ts.lookupCalls++
			if got := r.URL.Query().Get("email"); got == "manager@example.com" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data":[{"id":"mgr-1","email":"manager@example.com"}],"page":{"next":""}}`))
				return
			}
			w.WriteHeader(ts.lookupResponse.status)
			_, _ = w.Write([]byte(ts.lookupResponse.body))
		case r.Method == http.MethodPost && r.URL.Path == "/developer/v1/users/deferred":
			if ts.createRateLimit > 0 {
				w.Header().Set("X-RateLimit-Limit", strconv.Itoa(ts.createRateLimit))
			}
			ts.createCalls++
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			ts.createBodies = append(ts.createBodies, string(body))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"task-1"}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/developer/v1/departments/"):
			if ts.deptRateLimit > 0 {
				w.Header().Set("X-RateLimit-Limit", strconv.Itoa(ts.deptRateLimit))
			}
			ts.deptIDCalls++
			w.WriteHeader(ts.deptByID.status)
			_, _ = w.Write([]byte(ts.deptByID.body))
		case r.Method == http.MethodGet && r.URL.Path == "/developer/v1/departments":
			ts.deptListCalls++
			w.WriteHeader(ts.deptList.status)
			_, _ = w.Write([]byte(ts.deptList.body))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/developer/v1/locations/"):
			ts.locIDCalls++
			w.WriteHeader(ts.locByID.status)
			_, _ = w.Write([]byte(ts.locByID.body))
		case r.Method == http.MethodGet && r.URL.Path == "/developer/v1/locations":
			ts.locListCalls++
			w.WriteHeader(ts.locList.status)
			_, _ = w.Write([]byte(ts.locList.body))
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
	return newUserBuilder(c, true)
}

// attributeTestInfo returns an account info with base profile fields plus extras.
func attributeTestInfo(t *testing.T, extras map[string]string) *v2.AccountInfo {
	t.Helper()
	profile := map[string]any{
		"email":      "jane@example.com",
		"first_name": "Jane",
		"last_name":  "Doe",
		"role":       "BUSINESS_USER",
	}
	for k, v := range extras {
		profile[k] = v
	}
	s, err := structpb.NewStruct(profile)
	if err != nil {
		t.Fatal(err)
	}
	return &v2.AccountInfo{Profile: s}
}

// createUserWireBody mirrors client.CreateUserRequest for decoding the POST body.
type createUserWireBody struct {
	Email           string  `json:"email"`
	DepartmentID    *string `json:"department_id"`
	DirectManagerID *string `json:"direct_manager_id"`
	LocationID      *string `json:"location_id"`
}

func mustCreateOnce(t *testing.T, builder *userBuilder, ts *attributeTestServer) {
	t.Helper()
	resp, _, _, err := builder.CreateAccount(context.Background(), attributeTestInfo(t, nil), nil)
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

func TestCreateAccountWithoutAttributesMakesNoLookupCalls(t *testing.T) {
	ts := &attributeTestServer{
		lookupResponse: noUsersLookup,
		deptByID:       attrDeptByID,
		deptList:       attrDeptListOne,
		locByID:        attrLocByID,
		locList:        attrLocListOne,
	}
	builder := newAttributeTestBuilder(t, ts)
	mustCreateOnce(t, builder, ts)

	var body map[string]any
	if err := json.Unmarshal([]byte(ts.createBodies[0]), &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"direct_manager_id", "department_id", "location_id"} {
		if _, present := body[key]; present {
			t.Fatalf("attribute-less create body must omit %q: %s", key, ts.createBodies[0])
		}
	}
	if ts.deptIDCalls != 0 || ts.deptListCalls != 0 || ts.locIDCalls != 0 || ts.locListCalls != 0 {
		t.Fatalf("attribute-less creation made lookup calls: deptID %d deptList %d locID %d locList %d", ts.deptIDCalls, ts.deptListCalls, ts.locIDCalls, ts.locListCalls)
	}
}

func TestCreateAccountWithAttributeIDs(t *testing.T) {
	ts := &attributeTestServer{
		lookupResponse: noUsersLookup,
		deptByID:       attrDeptByID,
		locByID:        attrLocByID,
	}
	builder := newAttributeTestBuilder(t, ts)

	resp, _, _, err := builder.CreateAccount(context.Background(), attributeTestInfo(t, map[string]string{
		"direct_manager_email": "manager@example.com",
		"department":           "dept-1",
		"location":             "loc-1",
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := resp.(*v2.CreateAccountResponse_ActionRequiredResult); !ok {
		t.Fatalf("expected ActionRequiredResult, got %T", resp)
	}
	if ts.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", ts.createCalls)
	}

	var wire createUserWireBody
	if err := json.Unmarshal([]byte(ts.createBodies[0]), &wire); err != nil {
		t.Fatal(err)
	}
	if wire.DirectManagerID == nil || *wire.DirectManagerID != "mgr-1" {
		t.Fatalf("direct_manager_id = %v, want mgr-1 (body: %s)", wire.DirectManagerID, ts.createBodies[0])
	}
	if wire.DepartmentID == nil || *wire.DepartmentID != "dept-1" {
		t.Fatalf("department_id = %v, want dept-1", wire.DepartmentID)
	}
	if wire.LocationID == nil || *wire.LocationID != "loc-1" {
		t.Fatalf("location_id = %v, want loc-1", wire.LocationID)
	}
}

func TestCreateAccountWithAttributeNames(t *testing.T) {
	ts := &attributeTestServer{
		lookupResponse: noUsersLookup,
		deptByID:       attrDeptByID404,
		deptList:       attrDeptListOne,
		locByID:        attrLocByID404,
		locList:        attrLocListOne,
	}
	builder := newAttributeTestBuilder(t, ts)

	if _, _, _, err := builder.CreateAccount(context.Background(), attributeTestInfo(t, map[string]string{
		"department": "Engineering",
		"location":   "Remote",
	}), nil); err != nil {
		t.Fatal(err)
	}
	var wire createUserWireBody
	if err := json.Unmarshal([]byte(ts.createBodies[0]), &wire); err != nil {
		t.Fatal(err)
	}
	if wire.DepartmentID == nil || *wire.DepartmentID != "dept-9" {
		t.Fatalf("department_id = %v, want dept-9", wire.DepartmentID)
	}
	if wire.LocationID == nil || *wire.LocationID != "loc-9" {
		t.Fatalf("location_id = %v, want loc-9", wire.LocationID)
	}
}

func TestCreateAccountAmbiguousDepartmentNameFailsClosed(t *testing.T) {
	ts := &attributeTestServer{
		lookupResponse: noUsersLookup,
		deptByID:       attrDeptByID404,
		deptList:       attrDeptListTwo,
	}
	builder := newAttributeTestBuilder(t, ts)

	_, _, _, err := builder.CreateAccount(context.Background(), attributeTestInfo(t, map[string]string{
		"department": "Engineering",
	}), nil)
	if err == nil {
		t.Fatal("expected ambiguous department error")
	}
	if code := grpcstatus.Code(err); code != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", code)
	}
	for _, id := range []string{"dept-1", "dept-2"} {
		if !strings.Contains(err.Error(), id) {
			t.Fatalf("error %q must list candidate ID %s", err, id)
		}
	}
	if ts.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", ts.createCalls)
	}
}

func TestCreateAccountUnmatchedLocationNameFailsClosed(t *testing.T) {
	ts := &attributeTestServer{
		lookupResponse: noUsersLookup,
		locByID:        attrLocByID404,
		locList:        attrLookupResponse{status: http.StatusOK, body: `{"data":[{"id":"loc-1","name":"HQ"}],"page":{"next":""}}`},
	}
	builder := newAttributeTestBuilder(t, ts)

	_, _, _, err := builder.CreateAccount(context.Background(), attributeTestInfo(t, map[string]string{
		"location": "Atlantis",
	}), nil)
	if err == nil {
		t.Fatal("expected unmatched location error")
	}
	if code := grpcstatus.Code(err); code != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", code)
	}
	if ts.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", ts.createCalls)
	}
}

func TestCreateAccountUnknownManagerEmailFailsClosed(t *testing.T) {
	ts := &attributeTestServer{
		lookupResponse: noUsersLookup,
	}
	builder := newAttributeTestBuilder(t, ts)

	_, _, _, err := builder.CreateAccount(context.Background(), attributeTestInfo(t, map[string]string{
		"direct_manager_email": "ghost@example.com",
	}), nil)
	if err == nil {
		t.Fatal("expected unknown manager error")
	}
	if code := grpcstatus.Code(err); code != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", code)
	}
	if !strings.Contains(err.Error(), "ghost@example.com") {
		t.Fatalf("error %q must name the email", err)
	}
	if ts.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", ts.createCalls)
	}
}

func TestCreateAccountLookupForbiddenPropagates(t *testing.T) {
	ts := &attributeTestServer{
		lookupResponse: noUsersLookup,
		deptByID:       attrDeptByID403,
	}
	builder := newAttributeTestBuilder(t, ts)

	_, _, _, err := builder.CreateAccount(context.Background(), attributeTestInfo(t, map[string]string{
		"department": "dept-1",
	}), nil)
	if err == nil {
		t.Fatal("expected upstream forbidden error to propagate")
	}
	if code := grpcstatus.Code(err); code != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", code)
	}
	if ts.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", ts.createCalls)
	}
}

func TestCreateAccountFlagOffWithFieldsSetFailsClosed(t *testing.T) {
	// The email short-circuit lookup runs first per D5; assert ZERO
	// department/location lookup calls hit the fake server.
	deptOrLoc := false
	ts := &attributeTestServer{
		lookupResponse: noUsersLookup,
		deptByID:       attrDeptByID,
		deptList:       attrDeptListOne,
		locByID:        attrLocByID,
		locList:        attrLocListOne,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/developer/v1/users":
			ts.lookupCalls++
			w.WriteHeader(ts.lookupResponse.status)
			_, _ = w.Write([]byte(ts.lookupResponse.body))
		case strings.HasPrefix(r.URL.Path, "/developer/v1/departments"):
			deptOrLoc = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[],"page":{"next":""}}`))
		case strings.HasPrefix(r.URL.Path, "/developer/v1/locations"):
			deptOrLoc = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[],"page":{"next":""}}`))
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
	builder := newUserBuilder(c, false)

	_, _, _, err = builder.CreateAccount(context.Background(), attributeTestInfo(t, map[string]string{
		"department": "Engineering",
	}), nil)
	if err == nil {
		t.Fatal("expected flag-off guard error")
	}
	if code := grpcstatus.Code(err); code != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", code)
	}
	if !strings.Contains(err.Error(), "provisioning-attributes") {
		t.Fatalf("error %q must name provisioning-attributes", err)
	}
	if deptOrLoc {
		t.Fatal("flag-off guard must make zero department/location lookup calls")
	}
	if ts.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", ts.createCalls)
	}
}

func TestCreateAccountRateLimitAnnotationPrecedence(t *testing.T) {
	// The resolution phase (ID probe) carries its own rate-limit data, but the
	// CreateUser annotation must keep final precedence in the returned annos.
	ts := &attributeTestServer{
		lookupResponse:  noUsersLookup,
		deptByID:        attrDeptByID,
		deptList:        attrDeptListOne,
		locByID:         attrLocByID,
		locList:         attrLocListOne,
		deptRateLimit:   100,
		createRateLimit: 200,
	}
	builder := newAttributeTestBuilder(t, ts)
	resp, _, annos, err := builder.CreateAccount(context.Background(), attributeTestInfo(t, map[string]string{
		"department": "dept-1",
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := resp.(*v2.CreateAccountResponse_ActionRequiredResult); !ok {
		t.Fatalf("expected ActionRequiredResult, got %T", resp)
	}
	var rl v2.RateLimitDescription
	found, err := annos.Pick(&rl)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected a rate-limit annotation on the CreateAccount result")
	}
	if rl.Limit != 200 {
		t.Fatalf("rate-limit annotation Limit = %d, want 200 (CreateUser must win)", rl.Limit)
	}
}
