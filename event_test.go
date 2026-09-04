package gobus_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/assurrussa/gobus"
)

const (
	firstEventHandlerName  = "first"
	secondEventHandlerName = "second"
)

func TestBus_PublishWithoutSubscribersSucceeds(t *testing.T) {
	bus := gobus.New()

	if err := bus.Publish(context.Background(), testEvent{value: "unused"}); err != nil {
		t.Fatalf("Publish() error = %v, want nil", err)
	}
}

func TestBus_PublishCallsAllSubscribersInOrderAndJoinsErrors(t *testing.T) {
	ctx := context.WithValue(context.Background(), eventContextKey{}, "context-value")
	bus := gobus.New()
	firstErr := errors.New(firstEventHandlerName)
	secondErr := errors.New(secondEventHandlerName)
	order := make([]string, 0, 3)

	bus.Subscribe(&testEventHandler{name: firstEventHandlerName, err: firstErr, order: &order})
	bus.Subscribe(&testEventHandler{name: secondEventHandlerName, err: secondErr, order: &order})
	bus.Subscribe(&testEventHandler{name: "third", order: &order})

	err := bus.Publish(ctx, testEvent{value: "event"})
	if !errors.Is(err, firstErr) {
		t.Fatalf("Publish() error = %v, want errors.Is(firstErr)", err)
	}
	if !errors.Is(err, secondErr) {
		t.Fatalf("Publish() error = %v, want errors.Is(secondErr)", err)
	}

	want := []string{"first:event", "second:event", "third:event"}
	if len(order) != len(want) {
		t.Fatalf("subscriber order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("subscriber order = %v, want %v", order, want)
		}
	}
}

func TestBus_SubscribeAddsDuplicateHandler(t *testing.T) {
	bus := gobus.New()
	handler := &countingEventHandler{}
	bus.Subscribe(handler)
	bus.Subscribe(handler)

	if err := bus.Publish(context.Background(), testEvent{}); err != nil {
		t.Fatalf("Publish() error = %v, want nil", err)
	}
	if got := handler.calls.Load(); got != 2 {
		t.Fatalf("handler calls = %d, want 2", got)
	}
}

func TestBus_EventTypesAndInstancesAreIsolated(t *testing.T) {
	ctx := context.Background()
	firstBus := gobus.New()
	secondBus := gobus.New()
	valueHandler := &countingEventHandler{}
	pointerHandler := &countingPointerEventHandler{}
	secondHandler := &countingEventHandler{}

	firstBus.Subscribe(valueHandler)
	firstBus.Subscribe(pointerHandler)
	secondBus.Subscribe(secondHandler)

	if err := firstBus.Publish(ctx, testEvent{}); err != nil {
		t.Fatalf("first value Publish() error = %v", err)
	}
	if err := firstBus.Publish(ctx, &testEvent{}); err != nil {
		t.Fatalf("first pointer Publish() error = %v", err)
	}
	if err := secondBus.Publish(ctx, testEvent{}); err != nil {
		t.Fatalf("second Publish() error = %v", err)
	}

	if got := valueHandler.calls.Load(); got != 1 {
		t.Fatalf("value handler calls = %d, want 1", got)
	}
	if got := pointerHandler.calls.Load(); got != 1 {
		t.Fatalf("pointer handler calls = %d, want 1", got)
	}
	if got := secondHandler.calls.Load(); got != 1 {
		t.Fatalf("second bus handler calls = %d, want 1", got)
	}
}

func TestBus_PublishUsesSubscriberSnapshot(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	first := &blockingEventHandler{started: started, release: release}
	second := &countingEventHandler{}
	bus.Subscribe(first)

	done := make(chan error, 1)
	go func() {
		done <- bus.Publish(ctx, testEvent{})
	}()

	<-started
	bus.Subscribe(second)
	close(release)

	if err := <-done; err != nil {
		t.Fatalf("first Publish() error = %v", err)
	}
	if got := second.calls.Load(); got != 0 {
		t.Fatalf("subscriber added during Publish called %d times, want 0", got)
	}

	if err := bus.Publish(ctx, testEvent{}); err != nil {
		t.Fatalf("second Publish() error = %v", err)
	}
	if got := second.calls.Load(); got != 1 {
		t.Fatalf("subscriber calls after next Publish = %d, want 1", got)
	}
}

func TestBus_PublishPropagatesPanicAndStopsFanOut(t *testing.T) {
	bus := gobus.New()
	after := &countingEventHandler{}
	bus.Subscribe(panicEventHandler{})
	bus.Subscribe(after)

	defer func() {
		if recovered := recover(); recovered != "event panic" {
			t.Fatalf("recovered = %v, want event panic", recovered)
		}
		if got := after.calls.Load(); got != 0 {
			t.Fatalf("subscriber after panic called %d times, want 0", got)
		}
	}()

	_ = bus.Publish(context.Background(), testEvent{})
}

func TestBus_ConcurrentSubscribeAndPublish(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	bus.Subscribe(&countingEventHandler{})

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			bus.Subscribe(&countingEventHandler{})
		}()
		go func() {
			defer wg.Done()
			if err := bus.Publish(ctx, testEvent{}); err != nil {
				t.Errorf("Publish() error = %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestBus_PublishAllocationBudget(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	bus.Subscribe(noopEventHandler{})

	allocations := testing.AllocsPerRun(allocationRuns, func() {
		if err := bus.Publish(ctx, testEvent{}); err != nil {
			t.Fatalf("Publish() error = %v", err)
		}
	})
	if allocations != 0 {
		t.Fatalf("Publish allocations = %v, want 0", allocations)
	}
}

func BenchmarkPublish(b *testing.B) {
	ctx := context.Background()
	bus := gobus.New()
	bus.Subscribe(noopEventHandler{})
	b.ResetTimer()
	for range b.N {
		_ = bus.Publish(ctx, testEvent{})
	}
}

func ExampleBus_Publish() {
	bus := gobus.New()
	bus.Subscribe(printEventHandler{})

	_ = bus.Publish(context.Background(), printableEvent{value: "published"})
	// Output: published
}

type eventContextKey struct{}

type testEvent struct {
	value string
}

type testEventHandler struct {
	name  string
	err   error
	order *[]string
}

func (h *testEventHandler) Execute(ctx context.Context, event testEvent) error {
	if ctx.Value(eventContextKey{}) != "context-value" {
		return errors.New("unexpected context")
	}
	*h.order = append(*h.order, h.name+":"+event.value)
	return h.err
}

type countingEventHandler struct {
	calls atomic.Int64
}

func (h *countingEventHandler) Execute(_ context.Context, _ testEvent) error {
	h.calls.Add(1)
	return nil
}

type countingPointerEventHandler struct {
	calls atomic.Int64
}

func (h *countingPointerEventHandler) Execute(_ context.Context, _ *testEvent) error {
	h.calls.Add(1)
	return nil
}

type blockingEventHandler struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (h *blockingEventHandler) Execute(_ context.Context, _ testEvent) error {
	select {
	case h.started <- struct{}{}:
	default:
	}
	<-h.release
	return nil
}

type panicEventHandler struct{}

func (panicEventHandler) Execute(_ context.Context, _ testEvent) error {
	panic("event panic")
}

type noopEventHandler struct{}

func (noopEventHandler) Execute(_ context.Context, _ testEvent) error {
	return nil
}

type printableEvent struct {
	value string
}

type printEventHandler struct{}

func (printEventHandler) Execute(_ context.Context, event printableEvent) error {
	_, _ = fmt.Fprintln(os.Stdout, event.value)
	return nil
}

func TestBus_ZeroValueIsUsable_Event(t *testing.T) {
	ctx := context.Background()
	var bus gobus.Bus

	// Publishing to zero-value bus with no subscribers succeeds.
	if err := bus.Publish(ctx, testEvent{value: "initial"}); err != nil {
		t.Fatalf("Publish error = %v, want nil", err)
	}

	// Subscribing on zero-value bus works.
	handler := &countingEventHandler{}
	bus.Subscribe(handler)

	if err := bus.Publish(ctx, testEvent{value: "delivered"}); err != nil {
		t.Fatalf("Publish error = %v, want nil", err)
	}
	if got := handler.calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
}

func TestBus_PublishInvalidRegistryEntry(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	key := reflect.TypeFor[testEvent]()
	bus.InjectInvalidEventSubscribers(key, "not-a-subscriber-slice")

	err := bus.Publish(ctx, testEvent{})
	if !errors.Is(err, gobus.ErrInvalidRegistryEntryForTest) {
		t.Fatalf("Publish error = %v, want ErrInvalidRegistryEntryForTest", err)
	}
}

func TestBus_SubscribeNilHandlerPanics(t *testing.T) {
	var bus gobus.Bus
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on nil handler, got nil")
		}
	}()
	bus.Subscribe[testEvent](nil)
}

func TestBus_SubscribeTypedNilHandlerPanics(t *testing.T) {
	var bus gobus.Bus
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on typed nil handler, got nil")
		}
	}()
	var handler *testEventHandler
	bus.Subscribe[testEvent](handler)
}
