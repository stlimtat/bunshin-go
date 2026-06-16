package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

// ErrNotFound is returned when a workflow or version is absent.
var ErrNotFound = errors.New("workflow: not found")

// ErrVersionConflict is returned when Activate references an unknown version.
var ErrVersionConflict = errors.New("workflow: version not found")

// StatusDraft marks a spec that has not yet been activated.
const StatusDraft = "draft"

// StatusActive marks the live version of a workflow.
const StatusActive = "active"

// Spec is the canonical in-memory representation of a YAML workflow definition.
type Spec struct {
	// Name identifies the workflow within a tenant.
	Name string `yaml:"name"`
	// Description is optional human-readable documentation.
	Description string `yaml:"description,omitempty"`
	// Nodes is the ordered list of execution vertices.
	// For linear pipelines the order determines execution sequence.
	Nodes []NodeSpec `yaml:"nodes"`

	// Version is the content-hash version string (set by Parse, not parsed from YAML).
	Version string `yaml:"-"`
	// Status is "draft" or "active" (set by Store, not parsed from YAML).
	Status string `yaml:"-"`
}

// NodeSpec describes one vertex in the workflow graph.
type NodeSpec struct {
	// ID uniquely identifies this node within the workflow.
	ID string `yaml:"id"`
	// Runnable specifies what this node executes.
	Runnable RunnableRef `yaml:"runnable"`
	// Next explicitly names the successor node.
	// When absent on a non-router node, the compiler derives it from list position.
	Next string `yaml:"next,omitempty"`
	// Router declares the routing logic for branching / cyclic graphs.
	// When present, Next is ignored.
	Router *RouterRef `yaml:"router,omitempty"`
}

// RunnableRef is a tagged union describing the executable unit for a node.
// Exactly one of Type == "llm", "tool", or "custom" is valid.
type RunnableRef struct {
	// Type is the node kind: "llm", "tool", or "custom".
	Type string `yaml:"type"`

	// --- llm fields ---

	// ProviderTier selects a provider by ModelTier (fast|smart|reasoning).
	// Mutually exclusive with ProviderID.
	ProviderTier string `yaml:"provider_tier,omitempty"`
	// ProviderID selects a specific named provider instance.
	// Mutually exclusive with ProviderTier.
	ProviderID string `yaml:"provider_id,omitempty"`
	// Prompt names a Fragment in the PromptBackend to render as the user message.
	Prompt string `yaml:"prompt,omitempty"`

	// --- tool / custom fields ---

	// Name is the tool or runnable name in the registry.
	Name string `yaml:"name,omitempty"`

	// --- I/O ---

	// InputKey reads State.Data[input_key] and passes the value as the node's
	// primary input. For llm nodes it is available as {{.Input}} in templates.
	// Defaults to the entire State.Data map when absent.
	InputKey string `yaml:"input_key,omitempty"`
	// OutputKey writes the node's output to State.Data[output_key].
	// When absent the output is discarded (pass-through state).
	OutputKey string `yaml:"output_key,omitempty"`
}

// RouterRef names the router factory and its configuration.
type RouterRef struct {
	// Type is the EIP router type or "custom".
	Type string `yaml:"type"`
	// Name is the registered name for type=="custom".
	Name string `yaml:"name,omitempty"`
	// Config passes type-specific parameters to the factory.
	// Keys are sorted before hashing to ensure deterministic version strings.
	Config map[string]any `yaml:"config,omitempty"`
}

// Parse decodes YAML bytes into a Spec, validates it, and sets its Version field.
func Parse(data []byte) (*Spec, error) {
	var s Spec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("workflow: parse: %w", err)
	}
	if s.Name == "" {
		return nil, fmt.Errorf("workflow: parse: name is required")
	}
	if len(s.Nodes) == 0 {
		return nil, fmt.Errorf("workflow: parse: at least one node is required")
	}

	// Validate nodes and collect IDs for Next-target checking.
	ids := make(map[string]struct{}, len(s.Nodes))
	for i, n := range s.Nodes {
		if n.ID == "" {
			return nil, fmt.Errorf("workflow: parse: node[%d] missing id", i)
		}
		if _, dup := ids[n.ID]; dup {
			return nil, fmt.Errorf("workflow: parse: duplicate node id %q", n.ID)
		}
		ids[n.ID] = struct{}{}
		if n.Runnable.Type == "" {
			return nil, fmt.Errorf("workflow: parse: node %q missing runnable.type", n.ID)
		}
	}
	// Validate explicit Next targets reference existing nodes.
	for _, n := range s.Nodes {
		if n.Next != "" && n.Next != "__end__" {
			if _, ok := ids[n.Next]; !ok {
				return nil, fmt.Errorf("workflow: parse: node %q next=%q references unknown node", n.ID, n.Next)
			}
		}
	}

	canonical, err := canonicalJSON(&s)
	if err != nil {
		return nil, fmt.Errorf("workflow: parse: canonicalise: %w", err)
	}
	s.Version = contentHash(canonical)
	return &s, nil
}

// canonicalJSON marshals the stable fields of spec to JSON with sorted map keys.
// JSON encoding in Go sorts map keys alphabetically, making it deterministic
// across backends regardless of map iteration order.
func canonicalJSON(s *Spec) ([]byte, error) {
	type canonical struct {
		Name        string     `json:"name"`
		Description string     `json:"description,omitempty"`
		Nodes       []NodeSpec `json:"nodes"`
	}
	return json.Marshal(canonical{
		Name:        s.Name,
		Description: s.Description,
		Nodes:       s.Nodes,
	})
}

// contentHash returns "sha256:" + first 32 hex chars of the SHA-256 digest.
// 32 hex chars = 128 bits, providing strong collision resistance.
func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])[:32]
}
