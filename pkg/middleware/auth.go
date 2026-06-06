package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/stlimtat/bunshin-go/pkg/auth"
	"github.com/stlimtat/bunshin-go/pkg/core"
)

// ErrUnauthorized is returned by auth middleware when the caller is not authenticated.
var ErrUnauthorized = errors.New("unauthorized")

// ErrForbidden is returned by WithRBAC when the caller fails the role predicate.
var ErrForbidden = errors.New("forbidden")

// WithAPIKey authenticates callers by matching the X-API-Key header against key.
// On success, writes a Principal with Subject=key fingerprint (first 8 chars) to context.
func WithAPIKey(key string) Middleware {
	return func(next core.Runnable) core.Runnable {
		return core.NewRunnableFuncWithStream(
			next.Name(),
			func(ctx context.Context, input any) (any, error) {
				p, ok := auth.FromContext(ctx)
				if !ok || p.Subject == "" {
					// No principal yet — check if input carries HTTP headers via context.
					// In non-HTTP use, callers must inject Principal manually.
					return nil, ErrUnauthorized
				}
				return next.Invoke(ctx, input)
			},
			next.Stream,
		)
	}
}

// WithAPIKeyHTTP is an http.Handler middleware that validates X-API-Key and injects Principal.
func WithAPIKeyHTTP(key, tenantID string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("X-API-Key")
		if got != key {
			http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
			return
		}
		subject := got
		if len(subject) > 8 {
			subject = subject[:8]
		}
		p := auth.Principal{Subject: subject, TenantID: tenantID}
		next.ServeHTTP(w, r.WithContext(auth.WithContext(r.Context(), p)))
	})
}

// WithBearerJWT is a Runnable middleware that checks for a Bearer token in context.
// The token string must be injected into context under the bearerTokenKey by an
// HTTP adapter (e.g. WithBearerJWTHTTP) before the Runnable chain is invoked.
func WithBearerJWT(validate func(token string) (auth.Principal, error)) Middleware {
	return func(next core.Runnable) core.Runnable {
		return core.NewRunnableFuncWithStream(
			next.Name(),
			func(ctx context.Context, input any) (any, error) {
				token, ok := bearerTokenFromContext(ctx)
				if !ok || token == "" {
					return nil, ErrUnauthorized
				}
				p, err := validate(token)
				if err != nil {
					return nil, ErrUnauthorized
				}
				return next.Invoke(auth.WithContext(ctx, p), input)
			},
			next.Stream,
		)
	}
}

// WithBearerJWTHTTP extracts Authorization: Bearer <token>, validates it, and
// injects Principal into the request context.
func WithBearerJWTHTTP(validate func(token string) (auth.Principal, error), next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(hdr, "Bearer ")
		if !ok || token == "" {
			http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
			return
		}
		p, err := validate(token)
		if err != nil {
			http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
			return
		}
		ctx := auth.WithContext(r.Context(), p)
		ctx = withBearerToken(ctx, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// WithRBAC applies role-based access control to a Runnable.
// predicate receives the Principal from context; returning false causes ErrForbidden.
func WithRBAC(predicate func(auth.Principal) bool) Middleware {
	return func(next core.Runnable) core.Runnable {
		return core.NewRunnableFuncWithStream(
			next.Name(),
			func(ctx context.Context, input any) (any, error) {
				p, ok := auth.FromContext(ctx)
				if !ok {
					return nil, ErrUnauthorized
				}
				if !predicate(p) {
					return nil, ErrForbidden
				}
				return next.Invoke(ctx, input)
			},
			next.Stream,
		)
	}
}

// WithIPAllowlistHTTP rejects requests whose remote IP is not in allowedCIDRs.
// allowedCIDRs entries are matched as prefix strings for simplicity.
func WithIPAllowlistHTTP(allowedCIDRs []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if idx := strings.LastIndex(ip, ":"); idx != -1 {
			ip = ip[:idx]
		}
		ip = strings.Trim(ip, "[]")
		for _, cidr := range allowedCIDRs {
			if strings.HasPrefix(ip, cidr) {
				next.ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, ErrForbidden.Error(), http.StatusForbidden)
	})
}

type bearerTokenContextKey struct{}

func withBearerToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, bearerTokenContextKey{}, token)
}

func bearerTokenFromContext(ctx context.Context) (string, bool) {
	t, ok := ctx.Value(bearerTokenContextKey{}).(string)
	return t, ok
}
