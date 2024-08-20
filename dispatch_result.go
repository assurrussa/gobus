package gobus

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"sync/atomic"
	"unsafe"
)

type Envelope[T ObjectOut] struct {
	Result T
	Error  error
}

// dataMapResultCommand List handlers.
type dataMapResultCommand[Q ObjectIn, T ObjectOut] map[string]ResultCommandExecutor[Q, T]

// dataResultCommand Current pointer for dataMapResultCommand.
var dataResultCommand unsafe.Pointer

func init() {
	InitResultCommand()
}

func InitResultCommand() {
	pointer := unsafe.Pointer(&dataMapResultCommand[any, any]{})
	atomic.StorePointer(&dataResultCommand, pointer)
}

func RegisterResult[Q ObjectIn, T ObjectOut](handler ResultCommandExecutor[Q, T]) {
	newData := maps.Clone(loadDataReadOnlyResultCommand[Q, T]())
	var q Q
	keyName := reflect.TypeOf(q).String()
	newData[keyName] = handler
	pointer := unsafe.Pointer(&newData)
	atomic.StorePointer(&dataResultCommand, pointer)
}

func DispatchResult[Q ObjectIn, T ObjectOut](ctx context.Context, dto Q) (T, error) {
	keyName := reflect.TypeOf(dto).String()
	handler, ok := loadDataReadOnlyResultCommand[Q, T]()[keyName]
	if !ok {
		var t T
		return t, fmt.Errorf("not found handler for %s", keyName)
	}

	return handler.Execute(ctx, dto)
}

func DispatchResultAsync[Q ObjectIn, T ObjectOut](ctx context.Context, dto Q) <-chan Envelope[T] {
	ch := make(chan Envelope[T], 1)
	go func() {
		defer close(ch)

		out, err := DispatchResult[Q, T](ctx, dto)
		if err != nil {
			ch <- Envelope[T]{Error: err}
			return
		}

		ch <- Envelope[T]{Result: out}
	}()

	return ch
}

func loadDataReadOnlyResultCommand[Q ObjectIn, T ObjectOut]() dataMapResultCommand[Q, T] {
	pointer := atomic.LoadPointer(&dataResultCommand)

	return *(*dataMapResultCommand[Q, T])(pointer)
}
