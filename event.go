package gobus

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
)

// Subscribe adds handler to the subscribers for events of type E.
// Each call adds a distinct subscription that lives for the lifetime of b.
// It panics if handler is nil.
func (b *Bus) Subscribe[E ObjectIn](handler EventExecutor[E]) {
	if isNil(handler) {
		panic("gobus: nil handler")
	}

	b.registerMu.Lock()
	defer b.registerMu.Unlock()

	current := b.events.Load()
	var newData map[reflect.Type]any
	if current != nil {
		newData = maps.Clone(*current)
	} else {
		newData = make(map[reflect.Type]any)
	}
	key := reflect.TypeFor[E]()
	var handlers []EventExecutor[E]
	if existing, ok := newData[key]; ok {
		if currentHandlers, ok := existing.([]EventExecutor[E]); ok {
			handlers = make([]EventExecutor[E], len(currentHandlers)+1)
			copy(handlers, currentHandlers)
			handlers[len(currentHandlers)] = handler
		}
	}
	if handlers == nil {
		handlers = []EventExecutor[E]{handler}
	}
	newData[key] = handlers
	b.events.Store(&newData)
}

// Publish executes a snapshot of the subscribers for event's type in
// subscription order. It invokes every subscriber and joins returned errors.
// Publishing an event with no subscribers succeeds.
func (b *Bus) Publish[E ObjectIn](ctx context.Context, event E) error {
	key := reflect.TypeFor[E]()
	current := b.events.Load()
	if current == nil {
		return nil
	}

	value, ok := (*current)[key]
	if !ok {
		return nil
	}

	handlers, ok := value.([]EventExecutor[E])
	if !ok {
		return fmt.Errorf("%w for %s", errInvalidRegistryEntry, key)
	}

	var publishErrors []error
	for _, handler := range handlers {
		if err := handler.Execute(ctx, event); err != nil {
			publishErrors = append(publishErrors, err)
		}
	}

	return errors.Join(publishErrors...)
}
