package gobus

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"sync/atomic"
	"unsafe"
)

// dataMapCommand List handlers.
type dataMapCommand[Q ObjectIn] map[string]CommandExecutor[Q]

// dataCommand Current pointer for dataMapCommand.
var dataCommand unsafe.Pointer

func init() {
	InitCommand()
}

func InitCommand() {
	pointer := unsafe.Pointer(&dataMapCommand[any]{})
	atomic.StorePointer(&dataCommand, pointer)
}

func Register[Q ObjectIn](handler CommandExecutor[Q]) {
	newData := maps.Clone(loadDataCommandReadOnlyCommand[Q]())
	var q Q
	keyName := reflect.TypeOf(q).String()
	newData[keyName] = handler
	pointer := unsafe.Pointer(&newData)
	atomic.StorePointer(&dataCommand, pointer)
}

func Dispatch[Q ObjectIn](ctx context.Context, dto Q) error {
	keyName := reflect.TypeOf(dto).String()
	handler, ok := loadDataCommandReadOnlyCommand[Q]()[keyName]
	if !ok {
		return fmt.Errorf("not found handler for %s", keyName)
	}

	return handler.Execute(ctx, dto)
}

func DispatchAsync[Q ObjectIn](ctx context.Context, dto Q) <-chan error {
	ch := make(chan error, 1)

	go func() {
		defer close(ch)

		if err := Dispatch[Q](ctx, dto); err != nil {
			ch <- err
		}
	}()

	return ch
}

func loadDataCommandReadOnlyCommand[Q ObjectIn]() dataMapCommand[Q] {
	pointer := atomic.LoadPointer(&dataCommand)

	return *(*dataMapCommand[Q])(pointer)
}
