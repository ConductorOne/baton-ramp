package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func TestListDepartmentsPaginatesThroughNextToken(t *testing.T) {
	ctx := context.Background()
	var requests []*http.Request

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Clone(ctx))
		w.Header().Set("Content-Type", "application/json")
		switch len(requests) {
		case 1:
			_, _ = w.Write([]byte(`{"data":[{"id":"dept-1","name":"Engineering"}],"page":{"next":"` + server.URL + `/developer/v1/departments?start=cursor-1"}}`))
		case 2:
			_, _ = w.Write([]byte(`{"data":[{"id":"dept-2","name":"Finance"}],"page":{"next":""}}`))
		default:
			t.Fatalf("unexpected request %d: %s", len(requests), r.URL.RequestURI())
		}
	}))
	defer server.Close()

	c, err := New(ctx, Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	firstPage, _, err := c.ListDepartments(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(firstPage.Departments) != 1 || firstPage.Departments[0].ID != "dept-1" {
		t.Fatalf("first page departments = %+v", firstPage.Departments)
	}
	if firstPage.Pagination == "" {
		t.Fatal("expected first page to return pagination token")
	}
	secondPage, _, err := c.ListDepartments(ctx, firstPage.Pagination)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondPage.Departments) != 1 || secondPage.Departments[0].ID != "dept-2" {
		t.Fatalf("second page departments = %+v", secondPage.Departments)
	}
	if secondPage.Pagination != "" {
		t.Fatalf("pagination = %q, want empty", secondPage.Pagination)
	}

	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requests))
	}
	if requests[0].URL.Query().Get("page_size") != "100" {
		t.Fatalf("first page_size query = %q, want 100", requests[0].URL.Query().Get("page_size"))
	}
	if requests[1].URL.Query().Get("page_size") != "100" {
		t.Fatalf("second page_size query = %q, want 100", requests[1].URL.Query().Get("page_size"))
	}
}

func TestListLocationsPaginatesThroughNextToken(t *testing.T) {
	ctx := context.Background()
	var requests []*http.Request

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Clone(ctx))
		w.Header().Set("Content-Type", "application/json")
		switch len(requests) {
		case 1:
			_, _ = w.Write([]byte(`{"data":[{"id":"loc-1","name":"HQ","entity_id":"ent-1"}],"page":{"next":"` + server.URL + `/developer/v1/locations?start=cursor-1"}}`))
		case 2:
			_, _ = w.Write([]byte(`{"data":[{"id":"loc-2","name":"Remote","entity_id":"ent-2"}],"page":{"next":""}}`))
		default:
			t.Fatalf("unexpected request %d: %s", len(requests), r.URL.RequestURI())
		}
	}))
	defer server.Close()

	c, err := New(ctx, Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	firstPage, _, err := c.ListLocations(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(firstPage.Locations) != 1 || firstPage.Locations[0].ID != "loc-1" {
		t.Fatalf("first page locations = %+v", firstPage.Locations)
	}
	if firstPage.Pagination == "" {
		t.Fatal("expected first page to return pagination token")
	}
	secondPage, _, err := c.ListLocations(ctx, firstPage.Pagination)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondPage.Locations) != 1 || secondPage.Locations[0].ID != "loc-2" {
		t.Fatalf("second page locations = %+v", secondPage.Locations)
	}
	if secondPage.Pagination != "" {
		t.Fatalf("pagination = %q, want empty", secondPage.Pagination)
	}

	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requests))
	}
	if requests[0].URL.Query().Get("page_size") != "100" {
		t.Fatalf("first page_size query = %q, want 100", requests[0].URL.Query().Get("page_size"))
	}
	if requests[1].URL.Query().Get("page_size") != "100" {
		t.Fatalf("second page_size query = %q, want 100", requests[1].URL.Query().Get("page_size"))
	}
}

func TestGetDepartmentErrorPath(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
	defer server.Close()

	c, err := New(ctx, Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	resp, _, err := c.GetDepartment(ctx, "dept-missing")
	if err == nil {
		t.Fatal("expected error for 404 department GET")
	}
	if resp != nil {
		t.Fatal("expected nil department on error")
	}
	if code := grpcstatus.Code(err); code != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", code)
	}
}

func TestGetLocationErrorPath(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
	defer server.Close()

	c, err := New(ctx, Token{AccessToken: "token"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	resp, _, err := c.GetLocation(ctx, "loc-missing")
	if err == nil {
		t.Fatal("expected error for 404 location GET")
	}
	if resp != nil {
		t.Fatal("expected nil location on error")
	}
	if code := grpcstatus.Code(err); code != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", code)
	}
}
