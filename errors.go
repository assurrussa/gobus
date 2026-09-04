package gobus

import (
	"errors"
	"fmt"
)

// ErrHandlerNotFound is returned when a bus has no handler for the requested types.
var ErrHandlerNotFound = errors.New("not found handler")

// ErrHandlerGoexit is reported when an asynchronous handler terminates via runtime.Goexit.
var ErrHandlerGoexit = errors.New("gobus: handler exited via runtime.Goexit")

// errInvalidRegistryEntry is returned when an internal registry entry does not
// match the expected executor contract.
var errInvalidRegistryEntry = errors.New("invalid registry entry")

// PanicError reports a panic recovered while an asynchronous handler was executing.
type PanicError struct {
	Value any
	Stack string
}

// Error implements error.
func (e *PanicError) Error() string {
	return fmt.Sprintf("gobus: handler panic: %v", e.Value)
}

// Unwrap exposes a recovered error value to errors.Is and errors.As.
func (e *PanicError) Unwrap() error {
	err, _ := e.Value.(error)
	return err
}
