# GoBus

GoBus is a small, type-safe, in-process command, query, and event dispatcher for Go 1.27+.
It provides explicit, isolated bus instances without package-level state.

## Features

- Independent `*Bus` instances that can be created in a composition root and injected into services.
- Type-safe generic methods with inference for registration and command dispatch.
- Commands without a result and queries with a typed result.
- Events with zero or more ordered subscribers.
- Synchronous and asynchronous dispatch.
- Concurrent registration and dispatch: registrations use immutable copy-on-write snapshots, while dispatch performs an atomic load and lookup without locking.
- Re-registering the same command type, or the same query/result pair, replaces its handler.
- Query handlers are keyed by both input and output type, preventing an incompatible unsafe call.
- A sentinel `ErrHandlerNotFound` error that works with `errors.Is`.
- The runtime package imports only the Go standard library.
- An optional bounded async runtime with explicit queues, worker limits, and shutdown.
- A minimal core surface that can be extended with handler decorators.

GoBus is intentionally an in-process dispatcher, not a message broker. Event
pub/sub and managed asynchronous execution remain in memory. GoBus does not provide
persistence, retries, delivery after process termination, acknowledgements, or
distributed routing.

## Requirements and installation

GoBus requires Go 1.27 or later because its API uses generic methods.

The instance-based API is available starting with `v1.0.0`; events and the
managed async runtime are available starting with `v1.1.0`. `v0.9.1` contains
the legacy package-level API.

```sh
go get github.com/assurrussa/gobus@v1.1.0
```

Then import the package:

```go
import "github.com/assurrussa/gobus"
```

## Quick start

```go
package main

import (
	"context"
	"fmt"

	"github.com/assurrussa/gobus"
)

type CreateUser struct {
	Name string
}

type FindUser struct {
	Name string
}

type User struct {
	Name string
}

type UserCreated struct {
	Name string
}

type CreateUserHandler struct{}

func (CreateUserHandler) Execute(_ context.Context, command CreateUser) error {
	// Store the user.
	return nil
}

type FindUserHandler struct{}

func (FindUserHandler) Execute(_ context.Context, query FindUser) (User, error) {
	return User{Name: query.Name}, nil
}

type UserCreatedHandler struct{}

func (UserCreatedHandler) Execute(_ context.Context, event UserCreated) error {
	fmt.Println("created:", event.Name)
	return nil
}

func main() {
	ctx := context.Background()
	bus := gobus.New()

	bus.Register(CreateUserHandler{})
	bus.RegisterResult(FindUserHandler{})
	bus.Subscribe(UserCreatedHandler{})

	if err := bus.Dispatch(ctx, CreateUser{Name: "Amir"}); err != nil {
		panic(err)
	}

	user, err := bus.DispatchResult[User](ctx, FindUser{Name: "Amir"})
	if err != nil {
		panic(err)
	}

	fmt.Println(user.Name)

	if err := bus.Publish(ctx, UserCreated{Name: user.Name}); err != nil {
		panic(err)
	}
}
```

`Register`, `RegisterResult`, and `Dispatch` infer their type parameters from the
arguments. `DispatchResult` needs only the expected output type:

```go
bus.Register(commandHandler)
bus.RegisterResult(queryHandler)

err := bus.Dispatch(ctx, command)
result, err := bus.DispatchResult[Result](ctx, query)
```

Pointer and value command types are distinct and both are supported.

## Events

Event subscribers implement the typed `EventExecutor` contract:

```go
type EventExecutor[E any] interface {
	Execute(context.Context, E) error
}

bus.Subscribe(firstHandler)
bus.Subscribe(secondHandler)

err := bus.Publish(ctx, event)
```

`Subscribe` is append-only for the lifetime of a bus. Each call creates a
distinct subscription, including repeated registration of the same handler.
`Publish` loads one immutable subscriber snapshot and invokes every handler
sequentially in subscription order. It returns `nil` when there are no
subscribers and combines subscriber errors with `errors.Join`, so `errors.Is`
and `errors.As` remain usable.

The bus passes the provided context unchanged and does not stop fan-out merely
because it is cancelled. Subscribers decide how to handle cancellation. A panic
is not recovered by the synchronous core and stops the remaining fan-out.

## Asynchronous dispatch

The original asynchronous methods start one goroutine per call and immediately
return a receive-only channel:

```go
err := <-bus.DispatchAsync(ctx, command)
envelope := <-bus.DispatchResultAsync[Result](ctx, query)

result, err := envelope.Result, envelope.Error
```

Each channel receives exactly one value and is then closed. A successful
`DispatchAsync` sends `nil`. The provided context is passed to the handler unchanged;
cancellation behavior is therefore controlled by the handler.

These methods intentionally retain their v1.0.0 behavior. Use the optional
managed runtime when producers need bounded queueing and concurrency.

## Managed asynchronous execution

Import `github.com/assurrussa/gobus/async` for bounded in-memory execution of
commands, queries, and complete event publications:

```go
runtime, err := async.New(bus, async.QueueConfig{
	Capacity: 1024,
	Workers:  16,
})
if err != nil {
	return err
}

if err := runtime.AddQueue("reports", async.QueueConfig{
	Capacity: 64,
	Workers:  2,
}); err != nil {
	return err
}
if err := runtime.RouteCommand[GenerateReport]("reports"); err != nil {
	return err
}
if err := runtime.Start(); err != nil {
	return err
}

result, err := runtime.Submit(ctx, GenerateReport{ID: id})
if err != nil { // Admission failed.
	return err
}
if err := <-result; err != nil { // Handler failed.
	return err
}

shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
return runtime.Shutdown(shutdownCtx)
```

The default queue is mandatory and its capacity and worker count must be
positive. Additional queues have independent bounds. Commands route by input
type, queries by `(input, output)` pair, and events by event type; unconfigured
types use the default queue. Configuration is frozen by `Start`. `Capacity`
counts jobs waiting for workers; up to `Workers` additional jobs can execute,
so the maximum number of accepted unfinished jobs is `Capacity + Workers`.

`Submit`, `SubmitResult`, and `SubmitEvent` wait for capacity or context
cancellation. Their `Try` variants return `async.ErrQueueFull` immediately.
Admission errors are returned directly; an accepted job always sends exactly
one execution result to a buffered channel and closes it. The first context
argument controls admission. Without an option, it also controls execution:

```go
result, err := runtime.Submit(ctx, command)

// Equivalent to:
result, err = runtime.Submit(
	ctx,
	command,
	async.WithExecutionContext(ctx),
)
```

Use `WithExecutionContext` when queue admission and execution need independent
lifetimes. This applies equally to command, result, and event submissions and
their `Try` variants:

```go
admissionCtx, cancel := context.WithTimeout(request.Context(), 100*time.Millisecond)
defer cancel()

executionCtx := context.WithoutCancel(request.Context())
result, err := runtime.Submit(
	admissionCtx,
	SendEmail{UserID: id},
	async.WithExecutionContext(executionCtx),
)
```

Once admission succeeds, cancelling `admissionCtx` does not affect a distinct
execution context. The runtime stores the execution context with the job; if it
is already cancelled when a worker receives the job, the handler is skipped and
the completion channel receives that context error. `Try` submissions check the
admission context before their immediate enqueue attempt. Runtime shutdown
cancellation still reaches detached jobs.

An execution deadline starts when its context is created, not when a worker
starts the handler. Create timeouts inside the handler when they must measure
handler runtime rather than time spent waiting in the queue. The caller owns the
completion channel and should observe it when execution errors matter.

`Shutdown` immediately rejects new work, drains accepted jobs, and waits for
workers. If the first shutdown context expires, queued jobs that have not
started receive `async.ErrRuntimeShutdown`, running contexts are cancelled with
that cause, and `Shutdown` returns the context error. A handler that ignores
context cannot be forcibly terminated. Coordinate shutdown outside the
runtime's handlers: calling `Shutdown` from a job running on the same runtime
waits for the calling worker and can block until the shutdown context expires.

Each queue is FIFO, but completion order is guaranteed only with one worker.
Nested work that must finish before its parent should use synchronous `Bus`
methods or a different queue. Waiting for child work submitted to the same queue
can deadlock whenever all its workers are occupied by parent jobs waiting for
those children. `Runtime.Stats()` exposes queue configuration, depth, active
work, and cumulative accepted/completed/rejected counters.

The managed runtime attempts each accepted job at most once and does not persist
queued data. A process crash loses queued work. It does not implement retries,
acknowledgements, dead-letter queues, or durable delivery.

## Dependency injection

Create and populate a bus in the application composition root, then pass the same
instance to the services that need it:

```go
type Service struct {
	bus *gobus.Bus
}

func NewService(bus *gobus.Bus) *Service {
	return &Service{bus: bus}
}
```

Use a separate bus for another application boundary or for each test. Registrations
on one bus never affect another bus.

Generic methods cannot be represented as ordinary Go interface methods, so consumers
inject the concrete `*gobus.Bus`. The type should not be copied after first use.

## Extending the core

Cross-cutting behavior can be added by wrapping a `CommandExecutor` or
`ResultCommandExecutor` before registration. This keeps logging, metrics, tracing,
authorization, retries, and similar policies outside the dispatcher itself. For
example, an application-defined decorator can be registered like any other handler:

```go
decorated := mymiddleware.WithLogging(handler)
bus.Register(decorated)
```

Extensions should depend only on the exported handler contracts; the immutable
registry snapshots and unsafe dispatch machinery are internal implementation details.

## API behavior

- Create a bus with `gobus.New()`. The zero value is not supported.
- Pass the bus as `*gobus.Bus` and do not copy it after first use.
- Registration and dispatch are safe to run concurrently.
- A dispatch observes either the complete old registry snapshot or the complete new one.
- Re-registering a handler replaces the previous handler for that key.
- Missing handlers return an error matching `gobus.ErrHandlerNotFound` with `errors.Is`.
- Result handlers are keyed by `(query type, result type)`. Requesting the wrong result type returns `ErrHandlerNotFound` without invoking the registered handler.
- Handler errors are returned unchanged by synchronous and asynchronous dispatch.
- Events allow zero or more subscribers and join returned errors after calling all of them.
- Managed jobs resolve handlers and event subscriber snapshots when a worker executes them.

## Performance

Dispatch is optimized for the hot path. The current benchmark snapshot was collected
with Go 1.27 on an Apple M5 Pro using `-cpu=12` (`GOMAXPROCS=12`); values are medians
of five runs:

| Operation | Time | Memory | Allocations |
| --- | ---: | ---: | ---: |
| Register | 62.13 ns/op | 344 B/op | 3 allocs/op |
| Dispatch | 9.259 ns/op | 0 B/op | 0 allocs/op |
| RegisterResult | 75.08 ns/op | 360 B/op | 4 allocs/op |
| DispatchResult | 22.34 ns/op | 24 B/op | 1 alloc/op |
| Publish (one subscriber) | 9.861 ns/op | 0 B/op | 0 allocs/op |
| Managed Runtime Submit | 550.9 ns/op | 440 B/op | 9 allocs/op |

Run the same 12-CPU benchmark suite locally with:

```sh
make bench-all
```

Exact timings depend on the machine. Allocation regression tests enforce the more
important hot-path contract: command dispatch remains at `0 allocs/op`, while result
dispatch does not exceed `1 alloc/op`.

## Verification

Run the complete repository gate with:

```sh
make check
```

It includes formatting, build, vet, lint, tests, race detection, checkptr,
coverage, and an isolated consumer build. `golangci-lint` is an external
prerequisite; use v2.13.1 or newer built with Go 1.27 support. GitHub Actions
installs the pinned linter and runs this same gate.

The local consumer check validates the current checkout. Release validation must also
test a clean consumer against the published version without a local `replace` directive:

```sh
make test-consumer-release VERSION=vX.Y.Z
```

For migration from the legacy package-level API, see [MIGRATION.md](MIGRATION.md).

## License

GoBus is released under the [MIT License](LICENSE).
