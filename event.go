package gobus

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"sync/atomic"
	"unsafe"
)

type dataMapEvent[E ObjectIn] map[reflect.Type][]EventExecutor[E]

// Subscribe adds handler to the subscribers for events of type E.
// Each call adds a distinct subscription that lives for the lifetime of b.
func (b *Bus) Subscribe[E ObjectIn](handler EventExecutor[E]) {
	b.registerMu.Lock()
	defer b.registerMu.Unlock()

	newData := maps.Clone(b.loadDataEventReadOnly[E]())
	key := reflect.TypeFor[E]()
	current := newData[key]
	handlers := make([]EventExecutor[E], len(current)+1)
	copy(handlers, current)
	handlers[len(current)] = handler
	newData[key] = handlers
	pointer := unsafe.Pointer(&newData)
	atomic.StorePointer(&b.dataEvent, pointer)
}

// Publish executes a snapshot of the subscribers for event's type in
// subscription order. It invokes every subscriber and joins returned errors.
// Publishing an event with no subscribers succeeds.
func (b *Bus) Publish[E ObjectIn](ctx context.Context, event E) error {
	key := reflect.TypeFor[E]()
	handlers := b.loadDataEventReadOnly[E]()[key]

	var publishErrors []error
	for _, handler := range handlers {
		if err := handler.Execute(ctx, event); err != nil {
			publishErrors = append(publishErrors, err)
		}
	}

	return errors.Join(publishErrors...)
}

func (b *Bus) loadDataEventReadOnly[E ObjectIn]() dataMapEvent[E] {
	pointer := atomic.LoadPointer(&b.dataEvent)

	return *(*dataMapEvent[E])(pointer)
}
