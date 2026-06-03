package llm

// googleWirePart is a content part in the Gemini wire format.
// Currently text-only. Inline binary data (audio, video, document) requires
// an inlineData or fileData structure not yet implemented.
type googleWirePart struct {
	Text string `json:"text"`
}

// googleWireContent is a content block (role + parts) in the Gemini wire format.
type googleWireContent struct {
	Role  string           `json:"role,omitempty"`
	Parts []googleWirePart `json:"parts"`
}

// googleWireRequest is the JSON body for generateContent / streamGenerateContent.
type googleWireRequest struct {
	Contents          []googleWireContent `json:"contents"`
	SystemInstruction *googleWireContent  `json:"systemInstruction,omitempty"`
	GenerationConfig  *googleGenConfig    `json:"generationConfig,omitempty"`
}

// googleGenConfig holds generation parameters.
type googleGenConfig struct {
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
}

// googleWireResponse is the JSON body returned by generateContent.
type googleWireResponse struct {
	Candidates []struct {
		Content struct {
			Parts []googleWirePart `json:"parts"`
			Role  string           `json:"role"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}
