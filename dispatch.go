package gobus

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

// Bus dispatches commands to handlers registered on this instance.
//
// A Bus is safe for concurrent registration and dispatch. Registrations replace
// the handler for the same command type and publish an immutable snapshot, so
// dispatch does not acquire locks. The zero value of Bus is ready to use. New
// returns a convenient initialized *Bus. A Bus must not be copied after first use.
type Bus struct {
	commands       atomic.Pointer[map[reflect.Type]any]
	resultCommands atomic.Pointer[map[reflect.Type]any]
	events         atomic.Pointer[map[reflect.Type]any]
	registerMu     sync.Mutex
}

// New returns an initialized Bus with isolated handler registries.
func New() *Bus {
	return &Bus{}
}

// Register associates a command type with a handler on b, replacing any
// handler previously registered for that type. It panics if handler is nil.
func (b *Bus) Register[Q ObjectIn](handler CommandExecutor[Q]) {
	if isNil(handler) {
		panic("gobus: nil handler")
	}

	b.registerMu.Lock()
	defer b.registerMu.Unlock()

	current := b.commands.Load()
	var newData map[reflect.Type]any
	if current != nil {
		newData = maps.Clone(*current)
	} else {
		newData = make(map[reflect.Type]any)
	}
	key := reflect.TypeFor[Q]()
	newData[key] = handler
	b.commands.Store(&newData)
}

// Dispatch executes the handler registered for dto's type.
func (b *Bus) Dispatch[Q ObjectIn](ctx context.Context, dto Q) error {
	key := reflect.TypeFor[Q]()
	current := b.commands.Load()
	if current == nil {
		return fmt.Errorf("%w for %s", ErrHandlerNotFound, key)
	}

	value, ok := (*current)[key]
	if !ok {
		return fmt.Errorf("%w for %s", ErrHandlerNotFound, key)
	}

	handler, ok := value.(CommandExecutor[Q])
	if !ok {
		return fmt.Errorf("%w for %s", errInvalidRegistryEntry, key)
	}

	return handler.Execute(ctx, dto)
}

// DispatchAsync executes the handler registered for dto's type asynchronously in
// an unmanaged goroutine. The returned channel yields exactly one error, nil on
// success, *PanicError if the handler panics, or ErrHandlerGoexit if the handler
// exits via runtime.Goexit, and then closes. For managed execution with queues,
// concurrency limits, and backpressure, use async.Runtime.
func (b *Bus) DispatchAsync[Q ObjectIn](ctx context.Context, dto Q) <-chan error {
	ch := make(chan error, 1)

	go func() {
		var normalReturn bool
		defer close(ch)
		defer func() {
			if !normalReturn {
				if r := recover(); r != nil {
					ch <- &PanicError{
						Value: r,
						Stack: string(debug.Stack()),
					}
				} else {
					ch <- ErrHandlerGoexit
				}
			}
		}()

		err := b.Dispatch(ctx, dto)
		normalReturn = true
		ch <- err
	}()

	return ch
}

func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return rv.IsNil()
	default:
		return false
	}
}
