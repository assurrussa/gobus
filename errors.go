package gobus

import "errors"

// ErrHandlerNotFound is returned when a bus has no handler for the requested types.
var ErrHandlerNotFound = errors.New("not found handler")
