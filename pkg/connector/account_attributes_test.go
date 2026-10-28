package connector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/conductorone/baton-ramp/pkg/client"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// attrTestServer drives resolveCreationAttributes against a fake Ramp API.
// Routes: departments/{id}, departments, locations/{id}, locations, users?email=.
type attrTestServer struct {
	deptByIDStatus  int
	deptByIDBody    string
	deptListBody    string
	deptListCalls   int
	locByIDStatus   int
	locByIDBody     string
	locListBody     string
	locListCalls    int
	userLookupBody  string
	userLookupCalls int
}

func newAttrTestClient(t *testing.T, ts *attrTestServer) *client.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/developer/v1/departments/"):
			w.WriteHeader(ts.deptByIDStatus)
			_, _ = w.Write([]byte(ts.deptByIDBody))
		case r.Method == http.MethodGet && r.URL.Path == "/developer/v1/departments":
			ts.deptListCalls++
			_, _ = w.Write([]byte(ts.deptListBody))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/developer/v1/locations/"):
			w.WriteHeader(ts.locByIDStatus)
			_, _ = w.Write([]byte(ts.locByIDBody))
		case r.Method == http.MethodGet && r.URL.Path == "/developer/v1/locations":
			ts.locListCalls++
			_, _ = w.Write([]byte(ts.locListBody))
		case r.Method == http.MethodGet && r.URL.Path == "/developer/v1/users":
			if r.URL.Query().Get("email") == "" {
				t.Errorf("users lookup without email query: %s", r.URL.RawQuery)
			}
			ts.userLookupCalls++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(ts.userLookupBody))
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
	return c
}

func attrFields(t *testing.T, values map[string]string) map[string]*structpb.Value {
	t.Helper()
	fields := make(map[string]*structpb.Value)
	for k, v := range values {
		sv, err := structpb.NewValue(v)
		if err != nil {
			t.Fatal(err)
		}
		fields[k] = sv
	}
	return fields
}

func TestResolveAttributesFlagOffWithFieldsSet(t *testing.T) {
	ts := &attrTestServer{
		deptByIDStatus: http.StatusOK,
		deptByIDBody:   `{"id":"dept-1","name":"Engineering"}`,
	}
	c := newAttrTestClient(t, ts)

	fields := attrFields(t, map[string]string{"department": "dept-1"})
	attrs, err := resolveCreationAttributes(context.Background(), c, fields, false)
	if err == nil {
		t.Fatal("expected error when flag off with fields set")
	}
	if attrs != nil {
		t.Fatalf("attrs = %+v, want nil", attrs)
	}
	if code := grpcstatus.Code(err); code != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", code)
	}
	if !strings.Contains(err.Error(), "provisioning-attributes") {
		t.Fatalf("error %q must name provisioning-attributes", err)
	}
	if ts.deptListCalls != 0 || ts.locListCalls != 0 || ts.userLookupCalls != 0 {
		t.Fatalf("lookup calls: dept list %d loc list %d users %d, want all zero", ts.deptListCalls, ts.locListCalls, ts.userLookupCalls)
	}
}

func TestResolveAttributesPageCapExceeded(t *testing.T) {
	ts := &attrTestServer{
		deptByIDStatus: http.StatusNotFound,
		deptByIDBody:   `{"message":"not found"}`,
		// Always a next token: the walk must stop at maxAttrLookupPages.
		deptListBody: `{"data":[{"id":"dept-a","name":"Wrong"}],"page":{"next":""}}`,
	}
	// Serve a full-URL next token, like the real API's page.next.
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/developer/v1/departments/") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
			return
		}
		if r.URL.Path == "/developer/v1/departments" {
			ts.deptListCalls++
			_, _ = w.Write([]byte(`{"data":[{"id":"dept-a","name":"Wrong"}],"page":{"next":"` + server.URL + `/developer/v1/departments?start=cursor-` + strconv.Itoa(ts.deptListCalls) + `"}}`))
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	c, err := client.New(context.Background(), client.Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	fields := attrFields(t, map[string]string{"department": "Engineering"})
	_, err = resolveCreationAttributes(context.Background(), c, fields, true)
	if err == nil {
		t.Fatal("expected list-exhausted error")
	}
	if !strings.Contains(err.Error(), "list exhausted") {
		t.Fatalf("error %q must mention list exhaustion", err)
	}
	if ts.deptListCalls != maxAttrLookupPages {
		t.Fatalf("dept list calls = %d, want %d", ts.deptListCalls, maxAttrLookupPages)
	}
}

func TestResolveAttributesProbeInvalidArgumentFallsThroughToName(t *testing.T) {
	ts := &attrTestServer{
		// HTTP 400 maps to codes.InvalidArgument via uhttp GrpcCodeFromHTTPStatus.
		deptByIDStatus: http.StatusBadRequest,
		deptByIDBody:   `{"message":"UserInvalidInput"}`,
		deptListBody:   `{"data":[{"id":"dept-9","name":"Engineering"}],"page":{"next":""}}`,
	}
	c := newAttrTestClient(t, ts)

	fields := attrFields(t, map[string]string{"department": "Engineering"})
	attrs, err := resolveCreationAttributes(context.Background(), c, fields, true)
	if err != nil {
		t.Fatal(err)
	}
	if attrs == nil || attrs.DepartmentID != "dept-9" {
		t.Fatalf("attrs = %+v, want department dept-9", attrs)
	}
	if ts.deptListCalls != 1 {
		t.Fatalf("dept list calls = %d, want 1", ts.deptListCalls)
	}
}

func TestResolveAttributesNothingSetMakesNoCalls(t *testing.T) {
	ts := &attrTestServer{}
	c := newAttrTestClient(t, ts)

	attrs, err := resolveCreationAttributes(context.Background(), c, map[string]*structpb.Value{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if attrs != nil {
		t.Fatalf("attrs = %+v, want nil", attrs)
	}
	if ts.deptListCalls != 0 || ts.locListCalls != 0 || ts.userLookupCalls != 0 {
		t.Fatalf("lookup calls: dept list %d loc list %d users %d, want all zero", ts.deptListCalls, ts.locListCalls, ts.userLookupCalls)
	}
}

func TestResolveAttributesAmbiguousNameFailsClosed(t *testing.T) {
	ts := &attrTestServer{
		deptByIDStatus: http.StatusNotFound,
		deptByIDBody:   `{"message":"not found"}`,
		deptListBody:   `{"data":[{"id":"dept-1","name":"Engineering"},{"id":"dept-2","name":"Engineering"}],"page":{"next":""}}`,
	}
	c := newAttrTestClient(t, ts)

	fields := attrFields(t, map[string]string{"department": "engineering"})
	_, err := resolveCreationAttributes(context.Background(), c, fields, true)
	if err == nil {
		t.Fatal("expected ambiguous-name error")
	}
	if code := grpcstatus.Code(err); code != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", code)
	}
	for _, id := range []string{"dept-1", "dept-2"} {
		if !strings.Contains(err.Error(), id) {
			t.Fatalf("error %q must list matched ID %s", err, id)
		}
	}
}

func TestResolveAttributesUnknownManagerEmailFailsClosed(t *testing.T) {
	ts := &attrTestServer{
		userLookupBody: `{"data":[],"page":{"next":""}}`,
	}
	c := newAttrTestClient(t, ts)

	fields := attrFields(t, map[string]string{"direct_manager_email": "nobody@example.com"})
	_, err := resolveCreationAttributes(context.Background(), c, fields, true)
	if err == nil {
		t.Fatal("expected unresolved manager error")
	}
	if code := grpcstatus.Code(err); code != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", code)
	}
	if !strings.Contains(err.Error(), "nobody@example.com") {
		t.Fatalf("error %q must name the email", err)
	}
}

func TestResolveAttributesProbeForbiddenPropagates(t *testing.T) {
	ts := &attrTestServer{
		deptByIDStatus: http.StatusForbidden,
		deptByIDBody:   `{"message":"forbidden"}`,
	}
	c := newAttrTestClient(t, ts)

	fields := attrFields(t, map[string]string{"department": "dept-1"})
	_, err := resolveCreationAttributes(context.Background(), c, fields, true)
	if err == nil {
		t.Fatal("expected upstream error to propagate")
	}
	if code := grpcstatus.Code(err); code != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied (upstream code preserved)", code)
	}
	if strings.Contains(err.Error(), "no Ramp") {
		t.Fatalf("upstream error rewritten as no-match: %q", err)
	}
}

func TestResolveAttributesLocationByIDAndName(t *testing.T) {
	byID := &attrTestServer{
		locByIDStatus: http.StatusOK,
		locByIDBody:   `{"id":"loc-1","name":"HQ","entity_id":"ent-1"}`,
	}
	c := newAttrTestClient(t, byID)
	fields := attrFields(t, map[string]string{"location": "loc-1"})
	attrs, err := resolveCreationAttributes(context.Background(), c, fields, true)
	if err != nil {
		t.Fatal(err)
	}
	if attrs == nil || attrs.LocationID != "loc-1" {
		t.Fatalf("attrs = %+v, want location loc-1", attrs)
	}
	if byID.locListCalls != 0 {
		t.Fatalf("loc list calls = %d, want 0 (ID probe hit)", byID.locListCalls)
	}

	byName := &attrTestServer{
		locByIDStatus: http.StatusNotFound,
		locByIDBody:   `{"message":"not found"}`,
		locListBody:   `{"data":[{"id":"loc-2","name":"Remote"}],"page":{"next":""}}`,
	}
	c = newAttrTestClient(t, byName)
	fields = attrFields(t, map[string]string{"location": "Remote"})
	attrs, err = resolveCreationAttributes(context.Background(), c, fields, true)
	if err != nil {
		t.Fatal(err)
	}
	if attrs == nil || attrs.LocationID != "loc-2" {
		t.Fatalf("attrs = %+v, want location loc-2", attrs)
	}
}
