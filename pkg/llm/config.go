package llm

// ProviderID identifies a specific registered LLMProvider instance.
// It is the instance name, not the vendor — e.g. "openai-high-budget" or
// "anthropic-tenant-abc". Multiple instances of the same vendor coexist in
// ProviderRegistry under distinct ProviderIDs.
//
// The constants below are convenience defaults for single-instance deployments.
// Multi-instance deployments should use descriptive instance names.
type ProviderID string

const (
	ProviderAnthropic   ProviderID = "anthropic"
	ProviderAzureOpenAI ProviderID = "azure-openai"
	ProviderFake        ProviderID = "fake"
	ProviderGoogle      ProviderID = "google"
	ProviderOllama      ProviderID = "ollama"
	ProviderOpenAI      ProviderID = "openai"
)

// VendorID identifies the LLM vendor (the software, not the instance).
// Carried as the "vendor" tag on registry entries.
type VendorID string

const (
	VendorAnthropic   VendorID = "anthropic"
	VendorAzureOpenAI VendorID = "azure-openai"
	VendorGoogle      VendorID = "google"
	VendorOllama      VendorID = "ollama"
	VendorOpenAI      VendorID = "openai"
)

// Tags is a set of key-value metadata attached to a registered provider.
// Tags serve dual purpose: selection criteria (caller filters by tag) and
// descriptive metadata (vendor, tier, budget, region, tenant_tier, etc.).
type Tags map[string]string

// Tag returns a single-entry Tags map for use with ProviderRegistry.Select.
func Tag(key, value string) Tags {
	return Tags{key: value}
}

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
