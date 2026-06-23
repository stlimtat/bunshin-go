package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/stlimtat/bunshin-go/pkg/agent"
	"gopkg.in/yaml.v3"
)

// contentHashYAML marshals spec to canonical YAML and returns "sha256:" + hex[:32].
// Used by all store implementations to compute deterministic version strings.
func contentHashYAML(spec *agent.AgentSpec) (string, error) {
	data, err := yaml.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("yaml marshal: %w", err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])[:32], nil
}
