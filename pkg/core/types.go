package core

// StreamChunk carries one partial result from a streaming Runnable.
type StreamChunk struct {
	// Value is the partial output — type matches the Runnable's full output type.
	Value any
	// Err is non-nil only on the final chunk when the stream terminates with an error.
	Err error
}
