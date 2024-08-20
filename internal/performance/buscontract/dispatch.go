package bus

import (
	"context"
	"fmt"
	"maps"
	"sync/atomic"
	"unsafe"
)

type objectIn interface {
	Key() string
}

type objectOut interface {
	comparable
}

type Envelope[T objectOut] struct {
	Result T
	Error  error
}

// dataMap List handlers.
type dataMap[Q objectIn, T objectOut] map[string]Command[Q, T]

// data Current pointer for dataMap.
var data unsafe.Pointer

func init() {
	pointer := unsafe.Pointer(&dataMap[defaultIn, any]{})
	atomic.StorePointer(&data, pointer)
}

func Register[Q objectIn, T objectOut](q Q, handler Command[Q, T]) {
	newData := maps.Clone(loadDataReadOnly[Q, T]())
	newData[q.Key()] = handler
	pointer := unsafe.Pointer(&newData)
	atomic.StorePointer(&data, pointer)
}

func Dispatch[Q objectIn, T objectOut](ctx context.Context, dto Q) (T, error) {
	handler, ok := loadDataReadOnly[Q, T]()[dto.Key()]
	if !ok {
		var t T
		return t, fmt.Errorf("not found handler for %s", dto.Key())
	}

	return handler.Execute(ctx, dto)
}

func DispatchAsync[Q objectIn, T objectOut](ctx context.Context, dto Q) <-chan Envelope[T] {
	ch := make(chan Envelope[T], 1)
	go func() {
		defer close(ch)

		out, err := Dispatch[Q, T](ctx, dto)
		if err != nil {
			ch <- Envelope[T]{Error: err}
			return
		}

		ch <- Envelope[T]{Result: out}
	}()

	return ch
}

func loadDataReadOnly[Q objectIn, T objectOut]() dataMap[Q, T] {
	pointer := atomic.LoadPointer(&data)

	return *(*dataMap[Q, T])(pointer)
}

type defaultIn struct{}

func (defaultIn) Key() string {
	return ""
}
