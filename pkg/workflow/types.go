package workflow

import (
	"github.com/stlimtat/bunshin-go/pkg/llm"
	"github.com/stlimtat/bunshin-go/pkg/prompt"
	"github.com/stlimtat/bunshin-go/pkg/tools"
)

// Registries holds all runtime dependencies needed to compile a Spec.
type Registries struct {
	// LLM resolves provider instances for llm nodes.
	LLM *llm.ProviderRegistry
	// Tools resolves tool instances for tool nodes.
	Tools *tools.ToolRegistry
	// Custom resolves Go-defined Runnables for custom nodes and custom routers.
	Custom *RunnableRegistry
	// Prompts resolves Fragment templates for llm node prompts.
	Prompts prompt.PromptBackend
	// Routers resolves EIP router factories by type name.
	Routers *RouterRegistry
	// TenantID is passed to Prompts.Get when resolving fragment templates.
	TenantID string
}
