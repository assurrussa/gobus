package busevent

import (
	"context"
	"fmt"
	"reflect"
	"sync/atomic"
)

type ObjectIn any

type Envelope struct {
	Result any
	Error  error
}

type dataMap map[string]any

type CommandEvent struct {
	handlers atomic.Pointer[dataMap]
}

func NewCommandEvent() *CommandEvent {
	return &CommandEvent{
		handlers: atomic.Pointer[dataMap]{},
	}
}

func (c *CommandEvent) Register(dto ObjectIn, handler Command) {
	data := c.loadReadOnly()
	newData := make(dataMap)
	for k, v := range data {
		newData[k] = v
	}

	keyName := reflect.TypeOf(dto).String()
	newData[keyName] = handler
	c.handlers.Swap(&newData)
}

func (c *CommandEvent) Dispatch(ctx context.Context, dto ObjectIn, out any) error {
	keyName := reflect.TypeOf(dto).String()
	data := c.loadReadOnly()
	handler, ok := data[keyName]
	if !ok {
		return fmt.Errorf("not found handler for %s", keyName)
	}

	h, ok := handler.(Command)
	if !ok {
		return fmt.Errorf("handler is not implemented Command for %s", keyName)
	}

	res, err := h.Execute(ctx, dto)
	if err != nil {
		return fmt.Errorf("handler invoke %s: %w", keyName, err)
	}

	if out != nil {
		v := reflect.ValueOf(out)
		if v.Kind() == reflect.Ptr {
			v.Elem().Set(reflect.ValueOf(res))
		}
	}

	return nil
}

func (c *CommandEvent) DispatchAsync(ctx context.Context, dto any) <-chan Envelope {
	ch := make(chan Envelope, 1)
	go func() {
		defer close(ch)

		var out any
		err := c.Dispatch(ctx, dto, &out)
		if err != nil {
			ch <- Envelope{Error: err}
			return
		}

		ch <- Envelope{Result: out}
	}()

	return ch
}

func (c *CommandEvent) loadReadOnly() dataMap {
	if p := c.handlers.Load(); p != nil {
		return *p
	}

	return dataMap{}
}
