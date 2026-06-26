package store

import (
	"fmt"

	"github.com/stlimtat/bunshin-go/internal/hash"
	"github.com/stlimtat/bunshin-go/pkg/agent"
	"gopkg.in/yaml.v3"
)

// contentHashYAML marshals spec to canonical YAML and returns "sha256:" + hex[:32].
func contentHashYAML(spec *agent.AgentSpec) (string, error) {
	data, err := yaml.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("yaml marshal: %w", err)
	}
	return hash.Bytes(data), nil
}
