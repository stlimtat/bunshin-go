package llm

// ProviderID identifies an LLM provider.
type ProviderID string

const (
	ProviderAnthropic   ProviderID = "anthropic"
	ProviderAzureOpenAI ProviderID = "azure-openai"
	ProviderFake        ProviderID = "fake"
	ProviderGoogle      ProviderID = "google"
	ProviderOllama      ProviderID = "ollama"
	ProviderOpenAI      ProviderID = "openai"
)

// ModelTier classifies a model by capability vs cost trade-off.
type ModelTier string

const (
	TierFast      ModelTier = "fast"      // e.g. gpt-4o-mini, claude-haiku
	TierReasoning ModelTier = "reasoning" // e.g. o3, claude extended-thinking
	TierSmart     ModelTier = "smart"     // e.g. gpt-4o, claude-sonnet
)

// ModelID is the provider-specific model identifier.
type ModelID string

// ModelConfig fully specifies which model a workflow step should use.
type ModelConfig struct {
	Provider ProviderID
	Model    ModelID
	Tier     ModelTier
	// Endpoint overrides the default provider URL (Azure custom endpoints, Ollama local).
	Endpoint string
	// Region hints for cloud routing (AWS region, Azure region).
	Region string
}
