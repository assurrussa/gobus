package async

import (
	"errors"

	"github.com/assurrussa/gobus"
)

var (
	// ErrNilBus is returned when New receives a nil bus.
	ErrNilBus = errors.New("async: nil bus")
	// ErrNilContext is returned when submission or shutdown receives a nil context.
	ErrNilContext = errors.New("async: nil context")
	// ErrInvalidQueueConfig is returned for non-positive queue capacity or worker count.
	ErrInvalidQueueConfig = errors.New("async: invalid queue config")
	// ErrQueueExists is returned when a queue name is already configured.
	ErrQueueExists = errors.New("async: queue already exists")
	// ErrQueueNotFound is returned when a route names an unknown queue.
	ErrQueueNotFound = errors.New("async: queue not found")
	// ErrQueueFull is returned when a Try submission targets a full queue.
	ErrQueueFull = errors.New("async: queue full")
	// ErrInvalidSubmitOption is returned when a submission receives an invalid option.
	ErrInvalidSubmitOption = errors.New("async: invalid submit option")
	// ErrConfigFrozen is returned when configuration changes after Start or Shutdown.
	ErrConfigFrozen = errors.New("async: configuration is frozen")
	// ErrRuntimeNotStarted is returned when work is submitted before Start.
	ErrRuntimeNotStarted = errors.New("async: runtime not started")
	// ErrRuntimeStarted is returned when Start is called more than once.
	ErrRuntimeStarted = errors.New("async: runtime already started")
	// ErrRuntimeClosed is returned when work is submitted while the runtime is closing or closed.
	ErrRuntimeClosed = errors.New("async: runtime closed")
	// ErrRuntimeShutdown completes accepted jobs that cannot start after a shutdown deadline.
	ErrRuntimeShutdown = errors.New("async: runtime shutdown")
)

// PanicError reports a panic recovered while a managed job was executing.
// It is an alias for gobus.PanicError.
type PanicError = gobus.PanicError

// ErrHandlerGoexit is reported when an asynchronous handler terminates via runtime.Goexit.
// It is an alias for gobus.ErrHandlerGoexit.
var ErrHandlerGoexit = gobus.ErrHandlerGoexit
