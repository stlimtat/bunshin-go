package prompt

import "github.com/google/uuid"

// fragmentNS is the UUIDv5 namespace for bunshin fragment identity.
// Derived from the DNS namespace so it is reproducible across processes.
var fragmentNS = uuid.NewSHA1(uuid.NameSpaceDNS, []byte("bunshin-go.fragments"))

// slugUUID derives a stable UUIDv5 from tenantID and slug.
// Used by EmbedStore and FSStore so fragment identity is stable across
// process restarts. PostgresStore uses random UUIDv4 instead.
func slugUUID(tenantID, slug string) string {
	return uuid.NewSHA1(fragmentNS, []byte(tenantID+":"+slug)).String()
}
