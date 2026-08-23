package gobus_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/assurrussa/gobus"
)

func TestBus_EventRegistryConcurrentTypeIsolation(t *testing.T) {
	const (
		subscribers  = 32
		publications = 32
	)

	bus := gobus.New()
	valueEvent := registryEvent{value: 1}
	pointerEvent := &registryEvent{value: 2}
	otherEvent := registryOtherEvent{value: 3}
	valueHandler := newTypedCountingEventHandler(valueEvent)
	pointerHandler := newTypedCountingEventHandler(pointerEvent)
	otherHandler := newTypedCountingEventHandler(otherEvent)

	var subscribersWaitGroup sync.WaitGroup
	for range subscribers {
		subscribersWaitGroup.Add(3)
		go func() {
			defer subscribersWaitGroup.Done()
			bus.Subscribe(valueHandler)
		}()
		go func() {
			defer subscribersWaitGroup.Done()
			bus.Subscribe(pointerHandler)
		}()
		go func() {
			defer subscribersWaitGroup.Done()
			bus.Subscribe(otherHandler)
		}()
	}
	subscribersWaitGroup.Wait()

	publishErrors := make(chan error, publications*3)
	var publicationsWaitGroup sync.WaitGroup
	for range publications {
		publicationsWaitGroup.Add(3)
		go func() {
			defer publicationsWaitGroup.Done()
			publishErrors <- bus.Publish(context.Background(), valueEvent)
		}()
		go func() {
			defer publicationsWaitGroup.Done()
			publishErrors <- bus.Publish(context.Background(), pointerEvent)
		}()
		go func() {
			defer publicationsWaitGroup.Done()
			publishErrors <- bus.Publish(context.Background(), otherEvent)
		}()
	}
	publicationsWaitGroup.Wait()
	close(publishErrors)
	for err := range publishErrors {
		if err != nil {
			t.Fatalf("Publish() error = %v", err)
		}
	}

	wantCalls := int64(subscribers * publications)
	assertTypedEventHandler(t, "value", valueHandler, wantCalls)
	assertTypedEventHandler(t, "pointer", pointerHandler, wantCalls)
	assertTypedEventHandler(t, "other", otherHandler, wantCalls)
}

func FuzzBus_EventRegistryTypeIsolation(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 0})
	f.Add([]byte{255, 0, 128, 64, 32, 16, 1})

	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) < 7 {
			return
		}

		valueSubscribers := boundedFuzzCount(input[0], 8)
		pointerSubscribers := boundedFuzzCount(input[1], 8)
		otherSubscribers := boundedFuzzCount(input[2], 8)
		valuePublications := boundedFuzzCount(input[3], 16)
		pointerPublications := boundedFuzzCount(input[4], 16)
		otherPublications := boundedFuzzCount(input[5], 16)

		bus := gobus.New()
		valueEvent := registryEvent{value: uint64(input[0])}
		pointerEvent := &registryEvent{value: uint64(input[1])}
		if input[6]%2 == 1 {
			pointerEvent = nil
		}
		otherEvent := registryOtherEvent{value: uint64(input[2])}
		valueHandler := newTypedCountingEventHandler(valueEvent)
		pointerHandler := newTypedCountingEventHandler(pointerEvent)
		otherHandler := newTypedCountingEventHandler(otherEvent)

		subscribeTypedEventHandlers(bus, valueSubscribers, valueHandler)
		subscribeTypedEventHandlers(bus, pointerSubscribers, pointerHandler)
		subscribeTypedEventHandlers(bus, otherSubscribers, otherHandler)

		publishTypedEvents(t, bus, valuePublications, valueEvent)
		publishTypedEvents(t, bus, pointerPublications, pointerEvent)
		publishTypedEvents(t, bus, otherPublications, otherEvent)

		assertTypedEventHandler(t, "value", valueHandler, int64(valueSubscribers*valuePublications))
		assertTypedEventHandler(t, "pointer", pointerHandler, int64(pointerSubscribers*pointerPublications))
		assertTypedEventHandler(t, "other", otherHandler, int64(otherSubscribers*otherPublications))
	})
}

type registryEvent struct {
	value uint64
}

type registryOtherEvent struct {
	value uint64
}

type typedCountingEventHandler[E comparable] struct {
	expected   E
	calls      *atomic.Int64
	mismatches *atomic.Int64
}

func newTypedCountingEventHandler[E comparable](expected E) typedCountingEventHandler[E] {
	return typedCountingEventHandler[E]{
		expected:   expected,
		calls:      &atomic.Int64{},
		mismatches: &atomic.Int64{},
	}
}

func (h typedCountingEventHandler[E]) Execute(_ context.Context, event E) error {
	if event != h.expected {
		h.mismatches.Add(1)
	}
	h.calls.Add(1)
	return nil
}

func subscribeTypedEventHandlers[E comparable](
	bus *gobus.Bus,
	count int,
	handler typedCountingEventHandler[E],
) {
	for range count {
		bus.Subscribe(handler)
	}
}

func publishTypedEvents[E comparable](t *testing.T, bus *gobus.Bus, count int, event E) {
	t.Helper()
	for range count {
		if err := bus.Publish(context.Background(), event); err != nil {
			t.Fatalf("Publish() error = %v", err)
		}
	}
}

func assertTypedEventHandler[E comparable](
	t *testing.T,
	name string,
	handler typedCountingEventHandler[E],
	wantCalls int64,
) {
	t.Helper()
	if got := handler.calls.Load(); got != wantCalls {
		t.Fatalf("%s handler calls = %d, want %d", name, got, wantCalls)
	}
	if got := handler.mismatches.Load(); got != 0 {
		t.Fatalf("%s handler mismatches = %d, want 0", name, got)
	}
}

func boundedFuzzCount(value byte, maximum int) int {
	return int(value)%maximum + 1
}
