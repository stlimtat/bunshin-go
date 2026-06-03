// Package credentials provides a pluggable credential injection system
// for MCP clients and sandbox backends.
//
// Credentials are injected either via context (per-request) or via
// provider options (per-client). This allows:
//   - Multi-tenant deployments: different credentials per workflow run
//   - Secret manager integration: credentials fetched at call time, not startup
//   - Testing: stub credentials without touching real secret stores
//
// Usage — inject at request time:
//
//	ctx = credentials.WithCredential(ctx, "openai", credentials.APIKeyCredential("sk-..."))
//	result, err := mcpClient.CallTool(ctx, "search", input)
//
// Usage — inject at client construction:
//
//	client := mymcp.NewClient(&credentials.EnvProvider{Map: map[string]string{"openai": "OPENAI_API_KEY"}})
package credentials

import "context"

// Credential holds authentication material for a single service.
type Credential struct {
	// APIKey is a bearer token / API key.
	APIKey string
	// Token is an OAuth2 or JWT token.
	Token string
	// Headers are arbitrary key-value pairs injected into HTTP requests.
	Headers map[string]string
	// Extra is an extension point for non-standard auth (AWS SigV4, mTLS).
	Extra map[string]any
}

// APIKeyCredential returns a Credential containing only an API key.
func APIKeyCredential(key string) Credential {
	return Credential{APIKey: key}
}

// contextKey is the unexported context key type to avoid collisions.
type contextKey struct{ service string }

// WithCredential stores a credential for service in ctx.
func WithCredential(ctx context.Context, service string, cred Credential) context.Context {
	return context.WithValue(ctx, contextKey{service: service}, cred)
}

// FromContext retrieves the credential for service from ctx.
// Returns zero Credential and false if none is present.
func FromContext(ctx context.Context, service string) (Credential, bool) {
	c, ok := ctx.Value(contextKey{service: service}).(Credential)
	return c, ok
}

// Provider supplies credentials on demand. Implement this to integrate
// with AWS Secrets Manager, HashiCorp Vault, GCP Secret Manager, etc.
type Provider interface {
	// Get returns the credential for the named service.
	Get(ctx context.Context, service string) (Credential, error)
}

// EnvProvider reads credentials from environment variables.
// Map maps service name → env var name.
type EnvProvider struct {
	// Map maps service → env var that holds the API key.
	Map map[string]string
}

// Get returns a Credential whose APIKey is read from the environment variable
// configured for service. Returns empty credential if not configured.
func (p *EnvProvider) Get(_ context.Context, service string) (Credential, error) {
	varName, ok := p.Map[service]
	if !ok {
		return Credential{}, nil
	}
	// Lazy import to avoid adding an import cycle; read via os.Getenv equivalent.
	// Callers that need dynamic resolution should inject a real secret manager.
	return Credential{APIKey: envGet(varName)}, nil
}

// StaticProvider returns a fixed credential regardless of service name.
// Use for tests and single-service clients.
type StaticProvider struct {
	Cred Credential
}

func (p *StaticProvider) Get(_ context.Context, _ string) (Credential, error) {
	return p.Cred, nil
}
