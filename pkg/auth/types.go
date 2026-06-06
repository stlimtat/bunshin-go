package auth

// Principal is the authenticated caller identity. It is written to
// context.Context by auth middleware and read by WithRBAC and store
// implementations.
//
// TenantID scopes all store operations to the tenant's data — missing it is a
// bug in the store, not the caller. Subject identifies the user or API key.
// Roles drives role-predicate RBAC checks. Claims holds raw JWT claim values.
type Principal struct {
	Subject  string
	TenantID string
	Roles    []string
	Claims   map[string]any
}

// HasRole reports whether p holds the given role.
func (p Principal) HasRole(role string) bool {
	for _, r := range p.Roles {
		if r == role {
			return true
		}
	}
	return false
}
