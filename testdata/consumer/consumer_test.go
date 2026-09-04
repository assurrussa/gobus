package consumer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/assurrussa/gobus"
	busasync "github.com/assurrussa/gobus/async"
)

func TestPublicAPI(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	bus.Register(commandHandler{})
	bus.RegisterResult(queryHandler{})
	bus.Subscribe(eventHandler{})

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

	asyncResult := bus.DispatchAsync(ctx, command{id: 8})
	if err, ok := <-asyncResult; !ok || err != nil {
		t.Fatalf("DispatchAsync result = (%v, %t), want (nil, true)", err, ok)
	}
	if _, ok := <-asyncResult; ok {
		t.Fatal("DispatchAsync yielded more than one result")
	}

	if err := bus.Publish(ctx, event{id: 9}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	runtime, err := busasync.New(bus, busasync.QueueConfig{Capacity: 4, Workers: 1})
	if err != nil {
		t.Fatalf("async.New: %v", err)
	}
	if err := runtime.RouteEvent[event](busasync.DefaultQueueName); err != nil {
		t.Fatalf("RouteEvent: %v", err)
	}
	if err := runtime.Start(); err != nil {
		t.Fatalf("Runtime.Start: %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("Runtime.Shutdown: %v", err)
		}
	})
	executionOption := busasync.WithExecutionContext(
		context.WithValue(context.Background(), consumerExecutionContextKey{}, "managed"),
	)

	managedCommand, err := runtime.Submit(ctx, command{id: 10}, executionOption)
	if err != nil {
		t.Fatalf("Runtime.Submit: %v", err)
	}
	if err := <-managedCommand; err != nil {
		t.Fatalf("managed command result: %v", err)
	}
	tryManagedCommand, err := runtime.TrySubmit(ctx, command{id: 11}, executionOption)
	if err != nil {
		t.Fatalf("Runtime.TrySubmit: %v", err)
	}
	if err := <-tryManagedCommand; err != nil {
		t.Fatalf("try managed command result: %v", err)
	}

	managedQuery, err := runtime.SubmitResult[output](ctx, query{id: 12}, executionOption)
	if err != nil {
		t.Fatalf("Runtime.SubmitResult: %v", err)
	}
	if envelope := <-managedQuery; envelope.Error != nil || envelope.Result.id != 12 {
		t.Fatalf("managed query result = %+v, want id 12", envelope)
	}
	tryManagedQuery, err := runtime.TrySubmitResult[output](ctx, query{id: 13}, executionOption)
	if err != nil {
		t.Fatalf("Runtime.TrySubmitResult: %v", err)
	}
	if envelope := <-tryManagedQuery; envelope.Error != nil || envelope.Result.id != 13 {
		t.Fatalf("try managed query result = %+v, want id 13", envelope)
	}

	managedEvent, err := runtime.SubmitEvent(ctx, event{id: 14}, executionOption)
	if err != nil {
		t.Fatalf("Runtime.SubmitEvent: %v", err)
	}
	if err := <-managedEvent; err != nil {
		t.Fatalf("managed event result: %v", err)
	}
	tryManagedEvent, err := runtime.TrySubmitEvent(ctx, event{id: 15}, executionOption)
	if err != nil {
		t.Fatalf("Runtime.TrySubmitEvent: %v", err)
	}
	if err := <-tryManagedEvent; err != nil {
		t.Fatalf("try managed event result: %v", err)
	}

	if err := bus.Dispatch(ctx, missing{}); !errors.Is(err, gobus.ErrHandlerNotFound) {
		t.Fatalf("Dispatch missing error = %v, want ErrHandlerNotFound", err)
	}
	if _, err := bus.DispatchResult[string](ctx, query{id: 7}); !errors.Is(err, gobus.ErrHandlerNotFound) {
		t.Fatalf("DispatchResult wrong output error = %v, want ErrHandlerNotFound", err)
	}

	var cmd any = command{id: 7}
	if err := bus.Dispatch(ctx, cmd); !errors.Is(err, gobus.ErrHandlerNotFound) {
		t.Fatalf("Dispatch interface variable error = %v, want ErrHandlerNotFound", err)
	}

	partialErr := errors.New("partial")
	bus.RegisterResult(partialQueryHandler{err: partialErr})
	partialEnvelope := <-bus.DispatchResultAsync[output](ctx, partialQuery{id: 99})
	if partialEnvelope.Result.id != 99 || !errors.Is(partialEnvelope.Error, partialErr) {
		t.Fatalf("DispatchResultAsync partial = %+v, want id 99 and partialErr", partialEnvelope)
	}
}

func TestPublicAPI_ZeroValueBus(t *testing.T) {
	ctx := context.Background()
	var bus gobus.Bus

	if err := bus.Dispatch(ctx, command{id: 1}); !errors.Is(err, gobus.ErrHandlerNotFound) {
		t.Fatalf("Dispatch unregistered error = %v, want ErrHandlerNotFound", err)
	}
	if _, err := bus.DispatchResult[output](ctx, query{id: 1}); !errors.Is(err, gobus.ErrHandlerNotFound) {
		t.Fatalf("DispatchResult unregistered error = %v, want ErrHandlerNotFound", err)
	}
	if err := bus.Publish(ctx, event{id: 1}); err != nil {
		t.Fatalf("Publish unregistered error = %v, want nil", err)
	}

	bus.Register(commandHandler{})
	bus.RegisterResult(queryHandler{})
	bus.Subscribe(eventHandler{})

	if err := bus.Dispatch(ctx, command{id: 42}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	res, err := bus.DispatchResult[output](ctx, query{id: 42})
	if err != nil {
		t.Fatalf("DispatchResult: %v", err)
	}
	if res.id != 42 {
		t.Fatalf("DispatchResult id = %d, want 42", res.id)
	}
	if err := bus.Publish(ctx, event{id: 42}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
}

type partialQuery struct {
	id int
}

type partialQueryHandler struct {
	err error
}

func (h partialQueryHandler) Execute(_ context.Context, dto partialQuery) (output, error) {
	return output{id: dto.id}, h.err
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

type event struct {
	id int
}

type eventHandler struct{}

func (eventHandler) Execute(_ context.Context, _ event) error {
	return nil
}

type consumerExecutionContextKey struct{}
