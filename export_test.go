package gobus

import (
	"maps"
	"reflect"
)

// InjectInvalidCommandHandler injects an invalid handler into the commands registry for testing.
func (b *Bus) InjectInvalidCommandHandler(key reflect.Type, val any) {
	b.registerMu.Lock()
	defer b.registerMu.Unlock()

	current := b.commands.Load()
	var newData map[reflect.Type]any
	if current != nil {
		newData = maps.Clone(*current)
	} else {
		newData = make(map[reflect.Type]any)
	}
	newData[key] = val
	b.commands.Store(&newData)
}

// InjectInvalidResultCommandHandler injects an invalid handler into the resultCommands registry for testing.
func (b *Bus) InjectInvalidResultCommandHandler(key reflect.Type, val any) {
	b.registerMu.Lock()
	defer b.registerMu.Unlock()

	current := b.resultCommands.Load()
	var newData map[reflect.Type]any
	if current != nil {
		newData = maps.Clone(*current)
	} else {
		newData = make(map[reflect.Type]any)
	}
	newData[key] = val
	b.resultCommands.Store(&newData)
}

// InjectInvalidEventSubscribers injects an invalid value into the events registry for testing.
func (b *Bus) InjectInvalidEventSubscribers(key reflect.Type, val any) {
	b.registerMu.Lock()
	defer b.registerMu.Unlock()

	current := b.events.Load()
	var newData map[reflect.Type]any
	if current != nil {
		newData = maps.Clone(*current)
	} else {
		newData = make(map[reflect.Type]any)
	}
	newData[key] = val
	b.events.Store(&newData)
}

// ResultCommandTypeFor exposes resultCommandTypes reflect.Type for testing.
func ResultCommandTypeFor[Q ObjectIn, T ObjectOut]() reflect.Type {
	return reflect.TypeFor[resultCommandTypes[Q, T]]()
}

// ErrInvalidRegistryEntryForTest exposes errInvalidRegistryEntry for testing.
var ErrInvalidRegistryEntryForTest = errInvalidRegistryEntry
