package consumer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/assurrussa/gobus"
)

func TestPublicAPI(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	bus.Register(commandHandler{})
	bus.RegisterResult(queryHandler{})

	if err := bus.Dispatch(ctx, command{id: 7}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	result, err := bus.DispatchResult[output](ctx, query{id: 7})
	if err != nil {
		t.Fatalf("DispatchResult: %v", err)
	}
	if result.id != 7 {
		t.Fatalf("DispatchResult id = %d, want 7", result.id)
	}

	async := bus.DispatchAsync(ctx, command{id: 8})
	if err, ok := <-async; !ok || err != nil {
		t.Fatalf("DispatchAsync result = (%v, %t), want (nil, true)", err, ok)
	}
	if _, ok := <-async; ok {
		t.Fatal("DispatchAsync yielded more than one result")
	}

	if err := bus.Dispatch(ctx, missing{}); !errors.Is(err, gobus.ErrHandlerNotFound) {
		t.Fatalf("Dispatch missing error = %v, want ErrHandlerNotFound", err)
	}
	if _, err := bus.DispatchResult[string](ctx, query{id: 7}); !errors.Is(err, gobus.ErrHandlerNotFound) {
		t.Fatalf("DispatchResult wrong output error = %v, want ErrHandlerNotFound", err)
	}
}

type command struct {
	id int
}

type commandHandler struct{}

func (commandHandler) Execute(_ context.Context, _ command) error {
	return nil
}

type query struct {
	id int
}

type output struct {
	id int
}

type queryHandler struct{}

func (queryHandler) Execute(_ context.Context, dto query) (output, error) {
	return output{id: dto.id}, nil
}

type missing struct{}
