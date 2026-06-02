package llm

// Message is one turn in a conversation.
// native caches provider-specific representations to avoid redundant translation
// when the same message is sent to the same provider more than once.
type Message struct {
	Role  Role
	Parts []ContentPart

	// native maps ProviderID → provider-native wire format.
	// Populated lazily by LLMProvider.NativeMessages.
	native map[ProviderID]any
}

// NewTextMessage constructs a single-part text message.
func NewTextMessage(role Role, text string) Message {
	return Message{
		Role:   role,
		Parts:  []ContentPart{{Type: PartTypeText, Text: text}},
		native: make(map[ProviderID]any),
	}
}

// Text returns the concatenated text of all text parts.
func (m Message) Text() string {
	var out string
	for _, p := range m.Parts {
		if p.Type == PartTypeText {
			out += p.Text
		}
	}
	return out
}

// CacheNative stores a provider-native representation so it can be reused.
func (m *Message) CacheNative(id ProviderID, native any) {
	if m.native == nil {
		m.native = make(map[ProviderID]any)
	}
	m.native[id] = native
}

// Native returns the cached native representation for provider id, if any.
func (m *Message) Native(id ProviderID) (any, bool) {
	v, ok := m.native[id]
	return v, ok
}
