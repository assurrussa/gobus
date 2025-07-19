# GoBus

GoBus is a small, type-safe, in-process command and query dispatcher for Go 1.27+.
It provides explicit, isolated bus instances without package-level state.

## Features

- Independent `*Bus` instances that can be created in a composition root and injected into services.
- Type-safe generic methods with inference for registration and command dispatch.
- Commands without a result and queries with a typed result.
- Synchronous and asynchronous dispatch.
- Concurrent registration and dispatch: registrations use immutable copy-on-write snapshots, while dispatch performs an atomic load and lookup without locking.
- Re-registering the same command type, or the same query/result pair, replaces its handler.
- Query handlers are keyed by both input and output type, preventing an incompatible unsafe call.
- A sentinel `ErrHandlerNotFound` error that works with `errors.Is`.
- The runtime package imports only the Go standard library.
- A minimal public surface that can be extended with handler decorators.

GoBus is intentionally an in-process dispatcher, not a message broker. It does not
provide persistence, delivery guarantees, pub/sub, retries, or distributed routing.
Those policies belong in the application or in separate packages built around the
public handler contracts.

## Requirements and installation

GoBus requires Go 1.27 or later because its API uses generic methods.

The instance-based API documented here has not been published as a version tag yet.
`v0.9.1` contains the legacy package-level API. Until a Go 1.27 release is tagged,
consume a reviewed commit or pseudo-version. After publication, install the explicit
release version:

```sh
go get github.com/assurrussa/gobus@vX.Y.Z
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

type CreateUserHandler struct{}

func (CreateUserHandler) Execute(_ context.Context, command CreateUser) error {
	// Store the user.
	return nil
}

type FindUserHandler struct{}

func (FindUserHandler) Execute(_ context.Context, query FindUser) (User, error) {
	return User{Name: query.Name}, nil
}

func main() {
	ctx := context.Background()
	bus := gobus.New()

	bus.Register(CreateUserHandler{})
	bus.RegisterResult(FindUserHandler{})

	if err := bus.Dispatch(ctx, CreateUser{Name: "Amir"}); err != nil {
		panic(err)
	}

	user, err := bus.DispatchResult[User](ctx, FindUser{Name: "Amir"})
	if err != nil {
		panic(err)
	}

	fmt.Println(user.Name)
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

## Asynchronous dispatch

The asynchronous methods start the handler in a goroutine and immediately return a
receive-only channel:

```go
err := <-bus.DispatchAsync(ctx, command)
envelope := <-bus.DispatchResultAsync[Result](ctx, query)

result, err := envelope.Result, envelope.Error
```

Each channel receives exactly one value and is then closed. A successful
`DispatchAsync` sends `nil`. The provided context is passed to the handler unchanged;
cancellation behavior is therefore controlled by the handler.

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

## Performance

Dispatch is optimized for the hot path. The current benchmark snapshot was collected
with Go 1.27 on an Apple M5 Pro using `-cpu=12` (`GOMAXPROCS=12`); values are medians
of five runs:

| Operation | Time | Memory | Allocations |
| --- | ---: | ---: | ---: |
| Register | 69.92 ns/op | 344 B/op | 3 allocs/op |
| Dispatch | 9.683 ns/op | 0 B/op | 0 allocs/op |
| RegisterResult | 70.81 ns/op | 360 B/op | 4 allocs/op |
| DispatchResult | 22.26 ns/op | 24 B/op | 1 alloc/op |

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

It includes formatting, vet, lint, tests, race detection, checkptr, coverage, and an
isolated consumer build. `golangci-lint` is an external prerequisite; use v2.13.1 or
newer built with Go 1.27 support.

The local consumer check validates the current checkout. Release validation must also
test a clean consumer against the published version without a local `replace` directive:

```sh
make test-consumer-release VERSION=vX.Y.Z
```

For migration from the legacy package-level API, see [MIGRATION.md](MIGRATION.md).

## License

GoBus is released under the [MIT License](LICENSE).
