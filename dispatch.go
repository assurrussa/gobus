package gobus

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"sync"
	"sync/atomic"
	"unsafe"
)

// dataMapCommand List handlers.
type dataMapCommand[Q ObjectIn] map[reflect.Type]CommandExecutor[Q]

// Bus dispatches commands to handlers registered on this instance.
//
// A Bus is safe for concurrent registration and dispatch. Registrations replace
// the handler for the same command type and publish an immutable snapshot, so
// dispatch does not acquire locks. A Bus must be created with New and must not
// be copied after first use.
type Bus struct {
	dataCommand       unsafe.Pointer
	dataResultCommand unsafe.Pointer
	dataEvent         unsafe.Pointer
	registerMu        sync.Mutex
}

// New returns an initialized Bus with isolated handler registries.
func New() *Bus {
	command := dataMapCommand[any]{}
	resultCommand := dataMapResultCommand[any, any]{}
	event := dataMapEvent[any]{}

	return &Bus{
		dataCommand:       unsafe.Pointer(&command),
		dataResultCommand: unsafe.Pointer(&resultCommand),
		dataEvent:         unsafe.Pointer(&event),
	}
}

// Register associates a command type with a handler on b, replacing any
// handler previously registered for that type.
func (b *Bus) Register[Q ObjectIn](handler CommandExecutor[Q]) {
	b.registerMu.Lock()
	defer b.registerMu.Unlock()

	newData := maps.Clone(b.loadDataCommandReadOnly[Q]())
	key := reflect.TypeFor[Q]()
	newData[key] = handler
	pointer := unsafe.Pointer(&newData)
	atomic.StorePointer(&b.dataCommand, pointer)
}

// Dispatch executes the handler registered for dto's type.
func (b *Bus) Dispatch[Q ObjectIn](ctx context.Context, dto Q) error {
	key := reflect.TypeFor[Q]()
	handler, ok := b.loadDataCommandReadOnly[Q]()[key]
	if !ok {
		return fmt.Errorf("%w for %s", ErrHandlerNotFound, key)
	}

	return handler.Execute(ctx, dto)
}

// DispatchAsync executes the handler registered for dto's type asynchronously.
// The returned channel yields exactly one error, nil on success, and then closes.
func (b *Bus) DispatchAsync[Q ObjectIn](ctx context.Context, dto Q) <-chan error {
	ch := make(chan error, 1)

	go func() {
		defer close(ch)

		ch <- b.Dispatch(ctx, dto)
	}()

	return ch
}

func (b *Bus) loadDataCommandReadOnly[Q ObjectIn]() dataMapCommand[Q] {
	pointer := atomic.LoadPointer(&b.dataCommand)

	return *(*dataMapCommand[Q])(pointer)
}
