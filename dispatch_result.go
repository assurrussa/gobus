package gobus

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"sync/atomic"
	"unsafe"
)

// Envelope contains the result of an asynchronous query dispatch.
type Envelope[T ObjectOut] struct {
	Result T
	Error  error
}

// dataMapResultCommand List handlers.
type dataMapResultCommand[Q ObjectIn, T ObjectOut] map[reflect.Type]ResultCommandExecutor[Q, T]

type resultCommandTypes[Q ObjectIn, T ObjectOut] struct{}

// RegisterResult associates input and output types with a handler on b,
// replacing any handler previously registered for the same type pair.
func (b *Bus) RegisterResult[Q ObjectIn, T ObjectOut](handler ResultCommandExecutor[Q, T]) {
	b.registerMu.Lock()
	defer b.registerMu.Unlock()

	newData := maps.Clone(b.loadDataResultCommandReadOnly[Q, T]())
	key := reflect.TypeFor[resultCommandTypes[Q, T]]()
	newData[key] = handler
	pointer := unsafe.Pointer(&newData)
	atomic.StorePointer(&b.dataResultCommand, pointer)
}

// DispatchResult executes the handler registered for the input and output types.
func (b *Bus) DispatchResult[T ObjectOut, Q ObjectIn](ctx context.Context, dto Q) (T, error) {
	key := reflect.TypeFor[resultCommandTypes[Q, T]]()
	handler, ok := b.loadDataResultCommandReadOnly[Q, T]()[key]
	if !ok {
		var t T
		return t, fmt.Errorf(
			"%w for %s with result %s",
			ErrHandlerNotFound,
			reflect.TypeFor[Q](),
			reflect.TypeFor[T](),
		)
	}

	return handler.Execute(ctx, dto)
}

// DispatchResultAsync executes the result handler asynchronously. The returned
// channel yields exactly one envelope and then closes.
func (b *Bus) DispatchResultAsync[T ObjectOut, Q ObjectIn](ctx context.Context, dto Q) <-chan Envelope[T] {
	ch := make(chan Envelope[T], 1)
	go func() {
		defer close(ch)

		out, err := b.DispatchResult[T](ctx, dto)
		if err != nil {
			ch <- Envelope[T]{Error: err}
			return
		}

		ch <- Envelope[T]{Result: out}
	}()

	return ch
}

func (b *Bus) loadDataResultCommandReadOnly[Q ObjectIn, T ObjectOut]() dataMapResultCommand[Q, T] {
	pointer := atomic.LoadPointer(&b.dataResultCommand)

	return *(*dataMapResultCommand[Q, T])(pointer)
}
