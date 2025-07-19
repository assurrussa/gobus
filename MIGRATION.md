# Migrating to the instance-based Bus

The Go 1.27 API removes the package-level registries and functions. There is no default bus or compatibility layer.

## Requirements

- Go 1.27 or later.
- Construct each bus with `gobus.New()` and pass `*gobus.Bus` through the application's composition root.
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
