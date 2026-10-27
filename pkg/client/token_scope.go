package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/conductorone/baton-sdk/pkg/uhttp"
	"golang.org/x/oauth2"
	"google.golang.org/grpc/codes"
)

const (
	usersWriteScope   = "users:write"
	vendorsWriteScope = "vendors:write"
)

type scopeClass int

const (
	scopeClassRead scopeClass = iota
	scopeClassUsersWrite
	scopeClassVendorsWrite
)

type scopeClassKey struct{}

// TokenSources holds one token source per OAuth scope class. Nil write sources fall back to Read.
type TokenSources struct {
	Read         oauth2.TokenSource
	UsersWrite   oauth2.TokenSource
	VendorsWrite oauth2.TokenSource
	// ReadScopes names the scopes behind Read, used only in error messages.
	ReadScopes []string
}

func (s TokenSources) forClass(class scopeClass) oauth2.TokenSource {
	switch class {
	case scopeClassUsersWrite:
		if s.UsersWrite != nil {
			return s.UsersWrite
		}
	case scopeClassVendorsWrite:
		if s.VendorsWrite != nil {
			return s.VendorsWrite
		}
	case scopeClassRead:
	}
	return s.Read
}

func withScopeClass(ctx context.Context, class scopeClass) context.Context {
	return context.WithValue(ctx, scopeClassKey{}, class)
}

func scopeClassFrom(ctx context.Context) scopeClass {
	class, _ := ctx.Value(scopeClassKey{}).(scopeClass)
	return class
}

// classifyTokenError maps token endpoint failures to gRPC statuses, which uhttp keeps as-is.
// The message never includes the token endpoint response body.
func classifyTokenError(err error, class scopeClass, readScopes []string) error {
	var retrieveErr *oauth2.RetrieveError
	if !errors.As(err, &retrieveErr) {
		return fmt.Errorf("failed to get token: %w", err)
	}

	unauthorized := retrieveErr.Response != nil && retrieveErr.Response.StatusCode == http.StatusUnauthorized
	err = &redactedTokenError{err: retrieveErr}
	switch {
	case retrieveErr.ErrorCode == "invalid_scope":
		switch class {
		case scopeClassUsersWrite:
			return missingWriteScopeError(usersWriteScope, "creating, deactivating and reactivating users", err)
		case scopeClassVendorsWrite:
			return missingWriteScopeError(vendorsWriteScope, "granting and revoking vendor ownership", err)
		default:
			return uhttp.WrapErrors(codes.FailedPrecondition,
				fmt.Sprintf("Ramp OAuth app is missing one of the required read scopes: %s", strings.Join(readScopes, " ")),
				err)
		}
	case retrieveErr.ErrorCode == "invalid_client" || unauthorized:
		return uhttp.WrapErrors(codes.Unauthenticated, "Ramp rejected the OAuth client credentials", err)
	default:
		return fmt.Errorf("failed to get token: %w", err)
	}
}

func missingWriteScopeError(scope, operations string, err error) error {
	return uhttp.WrapErrors(codes.FailedPrecondition,
		fmt.Sprintf("Ramp OAuth app is not configured for scope %q; add it to the app in Ramp (Settings > Developers) to enable %s", scope, operations),
		err)
}

// redactedTokenError hides the token endpoint response body while keeping the *oauth2.RetrieveError reachable.
type redactedTokenError struct {
	err *oauth2.RetrieveError
}

func (e *redactedTokenError) Error() string {
	if e.err.ErrorCode != "" {
		return fmt.Sprintf("oauth2: %q", e.err.ErrorCode)
	}
	if e.err.Response != nil {
		return fmt.Sprintf("oauth2: cannot fetch token: %s", e.err.Response.Status)
	}
	return "oauth2: cannot fetch token"
}

func (e *redactedTokenError) Unwrap() error {
	return e.err
}
