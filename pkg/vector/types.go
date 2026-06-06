package vector

// Document is the unit stored in a VectorStore.
// Vector is populated by an llm.Embedder before Upsert.
// Metadata drives filter queries in Search.
type Document struct {
	ID       string
	Content  string
	Vector   []float32
	Metadata map[string]any
}

// SearchResult wraps a Document with its similarity score.
type SearchResult struct {
	Document
	// Score is the cosine similarity to the search query, in [0, 1].
	Score float32
}
