package core

import "fmt"

// RunnableError wraps an error with the name of the Runnable that produced it.
type RunnableError struct {
	Runnable string
	Cause    error
}

func (e *RunnableError) Error() string {
	return fmt.Sprintf("runnable %q: %v", e.Runnable, e.Cause)
}

func (e *RunnableError) Unwrap() error { return e.Cause }

// WrapError wraps err with the runnable name, or returns nil if err is nil.
func WrapError(runnableName string, err error) error {
	if err == nil {
		return nil
	}
	return &RunnableError{Runnable: runnableName, Cause: err}
}

// TypeMismatchError is returned when a Runnable receives an input of the wrong type.
type TypeMismatchError struct {
	Runnable string
	Got      any
}

func (e *TypeMismatchError) Error() string {
	return fmt.Sprintf("runnable %q: type mismatch: got %T", e.Runnable, e.Got)
}
