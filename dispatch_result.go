package gobus

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"runtime/debug"
)

// Envelope contains the result of an asynchronous query dispatch.
type Envelope[T ObjectOut] struct {
	Result T
	Error  error
}

type resultCommandTypes[Q ObjectIn, T ObjectOut] struct{}

// RegisterResult associates input and output types with a handler on b,
// replacing any handler previously registered for the same type pair.
// It panics if handler is nil.
func (b *Bus) RegisterResult[Q ObjectIn, T ObjectOut](handler ResultCommandExecutor[Q, T]) {
	if isNil(handler) {
		panic("gobus: nil handler")
	}

	b.registerMu.Lock()
	defer b.registerMu.Unlock()

	current := b.resultCommands.Load()
	var newData map[reflect.Type]any
	if current != nil {
		newData = maps.Clone(*current)
	} else {
		newData = make(map[reflect.Type]any)
	}
	key := reflect.TypeFor[resultCommandTypes[Q, T]]()
	newData[key] = handler
	b.resultCommands.Store(&newData)
}

// DispatchResult executes the handler registered for the input and output types.
func (b *Bus) DispatchResult[T ObjectOut, Q ObjectIn](ctx context.Context, dto Q) (T, error) {
	key := reflect.TypeFor[resultCommandTypes[Q, T]]()
	current := b.resultCommands.Load()
	if current == nil {
		var t T
		return t, fmt.Errorf(
			"%w for %s with result %s",
			ErrHandlerNotFound,
			reflect.TypeFor[Q](),
			reflect.TypeFor[T](),
		)
	}

	value, ok := (*current)[key]
	if !ok {
		var t T
		return t, fmt.Errorf(
			"%w for %s with result %s",
			ErrHandlerNotFound,
			reflect.TypeFor[Q](),
			reflect.TypeFor[T](),
		)
	}

	handler, ok := value.(ResultCommandExecutor[Q, T])
	if !ok {
		var t T
		return t, fmt.Errorf(
			"%w for %s with result %s",
			errInvalidRegistryEntry,
			reflect.TypeFor[Q](),
			reflect.TypeFor[T](),
		)
	}

	return handler.Execute(ctx, dto)
}

// DispatchResultAsync executes the result handler asynchronously in an unmanaged
// goroutine. The returned channel yields exactly one envelope containing the result
// and any error returned by the handler (or a *PanicError in Error if the handler panics),
// and then closes. For managed execution with queues, concurrency limits, and backpressure,
// use async.Runtime.
func (b *Bus) DispatchResultAsync[T ObjectOut, Q ObjectIn](ctx context.Context, dto Q) <-chan Envelope[T] {
	ch := make(chan Envelope[T], 1)
	go func() {
		defer close(ch)
		defer func() {
			if r := recover(); r != nil {
				ch <- Envelope[T]{
					Error: &PanicError{
						Value: r,
						Stack: string(debug.Stack()),
					},
				}
			}
		}()

		out, err := b.DispatchResult[T](ctx, dto)
		ch <- Envelope[T]{
			Result: out,
			Error:  err,
		}
	}()

	return ch
}
