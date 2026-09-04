# Migrating to the instance-based Bus

## Unreleased

These changes are available in the current source and are not part of `v1.2.0`:

- **`runtime.Goexit` Abnormal Exit Handling**: `DispatchAsync`, `DispatchResultAsync`, and managed `async.Runtime` detect `runtime.Goexit()` calls and report `gobus.ErrHandlerGoexit` (also exposed as `async.ErrHandlerGoexit`), ensuring completion channels receive an explicit failure instead of a zero-value or hanging. Managed queues automatically replace terminated workers to keep pool capacity intact.
- **Forced Shutdown Admission Coordination**: Late-finishing queue admissions during forced shutdown are rejected immediately or their pending jobs are drained and resolved with `async.ErrRuntimeShutdown` before waiting for busy workers, preventing orphaned jobs from remaining unresolved. Successful `Shutdown` calls and `StateClosed` also wait for concurrent pending-job cleanup to finish delivering all accepted results.

## Additions and behavior changes in v1.2.0

Version 1.2.0 replaces internal unsafe registry pointer manipulation with safe,
atomic copy-on-write typed snapshots (`map[reflect.Type]any`) protected by mutex
serialization during writes. Readers remain lock-free and observe consistent snapshots.

Key changes and additions:

- **Zero-value `Bus`**: The zero value `var bus gobus.Bus` is ready to use without calling `gobus.New()`. Registration, dispatch, and subscription methods safely initialize the first snapshot on demand.
- **Unmanaged Async Panic Recovery**: `DispatchAsync` and `DispatchResultAsync` now recover panics raised by handlers and report them as `*gobus.PanicError` through the returned channel or envelope error, preventing uncaught goroutine crashes.
- **Partial Query Result Preservation**: `DispatchResultAsync` and `async.Runtime.SubmitResult` / `TrySubmitResult` preserve partial results returned by query handlers alongside the error (`Envelope.Result` and `Envelope.Error` are both populated).
- **Nil Admission Context Protection**: Managed submission methods (`Submit`, `SubmitResult`, `SubmitEvent`, and their `Try` variants) and `Runtime.Shutdown` validate context arguments and return `async.ErrNilContext` when passed `nil`.
- **`async.PanicError` Alias**: `async.PanicError` is now a type alias for `gobus.PanicError`, ensuring uniform `errors.As` and `errors.Is` handling across managed and unmanaged asynchronous paths.

## Additions in v1.1.0

Version 1.1.0 is backward compatible with v1.0.0. Existing command and query
registrations do not require migration.

The root package adds synchronous in-process events:

```go
bus.Subscribe(userCreatedHandler)
err := bus.Publish(ctx, UserCreated{UserID: id})
```

An event may have zero or more subscribers. `Publish` calls a subscriber
snapshot sequentially in subscription order and joins returned errors. The
optional `github.com/assurrussa/gobus/async` package adds bounded asynchronous
execution with explicit worker and shutdown limits. Existing `DispatchAsync`
and `DispatchResultAsync` retain their v1.0.0 goroutine-per-call behavior.

By default, a managed submission uses one context for queue admission and
handler execution. `async.WithExecutionContext` lets background work retain a
separate execution context after successful admission. The option is supported
by command, result, and event submissions, including their `Try` variants. An
execution deadline starts when its context is created, not when a worker starts
the handler.

Queue capacity counts waiting jobs, with up to the configured worker count
executing in addition. Coordinate `Shutdown` outside handlers running on the
same runtime; otherwise the caller can wait for its own worker until the
shutdown context expires.

The v1.1.0 additions remain in-memory and do not provide persistence, retries,
or delivery after process termination.

The Go 1.27 API removes the package-level registries and functions. There is no default bus or compatibility layer.

## Requirements

- Go 1.27 or later.
- Construct each bus with `gobus.New()` or use the zero value `gobus.Bus`, and pass `*gobus.Bus` through the application's composition root.
- Do not copy a bus after first use.

## Registration and dispatch

Before:

```go
gobus.Register[CreateUser](createUserHandler)
gobus.RegisterResult[FindUser, User](findUserHandler)

err := gobus.Dispatch[CreateUser](ctx, command)
user, err := gobus.DispatchResult[FindUser, User](ctx, query)
```

After:

```go
bus := gobus.New()
bus.Register(createUserHandler)
bus.RegisterResult(findUserHandler)

err := bus.Dispatch(ctx, command)
user, err := bus.DispatchResult[User](ctx, query)
```

`Register` and `RegisterResult` infer all type arguments from the handler. Dispatch infers the input type from the DTO; result dispatch only needs the output type.

Each bus owns isolated handlers. Re-registering the same command type or `(input, output)` pair replaces the previous handler.

## Missing handlers

Missing command handlers and result type pairs can be detected without matching error strings:

```go
if errors.Is(err, gobus.ErrHandlerNotFound) {
	// handle missing registration
}
```

## Async calls

`DispatchAsync` and `DispatchResultAsync` each yield exactly one result and then close their channel. A successful `DispatchAsync` now sends `nil`; code ranging over the channel therefore receives one value instead of zero.

## Verification

Run `make check` with `golangci-lint` v2.13.1 or later built with Go 1.27. The isolated consumer check uses a local replacement and validates the checkout only. After publishing, verify the actual tag without a `replace` directive:

```sh
make test-consumer-release VERSION=vX.Y.Z
```
