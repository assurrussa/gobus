package gobus_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/assurrussa/gobus"
)

func TestBus_CommandExecutor_ExecuteComplex(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()

	err := bus.Dispatch(ctx, testIn{value: testValueIn, index: 1})
	checkError(t, err, gobus.ErrHandlerNotFound)

	err = <-bus.DispatchAsync(ctx, testIn{value: testValueIn, index: 1})
	checkError(t, err, gobus.ErrHandlerNotFound)

	bus.Register(&testHandleCommand{})
	bus.Register(&testHandleCommand2{})

	err = bus.Dispatch(ctx, testIn{value: testValueIn, index: 1})
	checkNoError(t, err)

	err = bus.Dispatch(ctx, &testIn{value: testValueIn, index: 1})
	checkNoError(t, err)

	wg := sync.WaitGroup{}

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Register(&testHandleCommand{})
		}()
	}

	for i := 0; i < 1000; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := bus.Dispatch(ctx, testIn{value: testValueIn, index: i})
			checkNoError(t, err)
		}()
	}

	for i := 0; i < 1000; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := <-bus.DispatchAsync(ctx, testIn{value: testValueIn, index: i})
			checkNoError(t, err)
		}()
	}

	for i := 1000; i < 2000; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			errExpect := errors.New("test error")
			err := bus.Dispatch(ctx, testIn{value: testValueIn, index: i, err: errExpect})
			checkError(t, err, errExpect)
		}()
	}

	wg.Wait()
}

func TestBus_ConcurrentRegistrationsPreserveDistinctHandlers(t *testing.T) {
	ctx := context.Background()

	for range 1000 {
		bus := gobus.New()
		start := make(chan struct{})
		wg := sync.WaitGroup{}
		wg.Add(2)

		go func() {
			defer wg.Done()
			<-start
			bus.Register(&testHandleCommand{})
		}()

		go func() {
			defer wg.Done()
			<-start
			bus.Register(&testHandleCommand2{})
		}()

		close(start)
		wg.Wait()

		checkNoError(t, bus.Dispatch(ctx, testIn{}))
		checkNoError(t, bus.Dispatch(ctx, &testIn{}))
	}
}

func TestBus_CommandHandlersAreIsolated(t *testing.T) {
	ctx := context.Background()
	firstBus := gobus.New()
	secondBus := gobus.New()
	firstCalled := false
	secondCalled := false
	firstBus.Register(&testTrackingCommandHandler{called: &firstCalled})
	secondBus.Register(&testTrackingCommandHandler{called: &secondCalled})

	checkNoError(t, firstBus.Dispatch(ctx, testIn{}))
	if !firstCalled {
		t.Fatal("first bus handler was not called")
	}
	if secondCalled {
		t.Fatal("second bus handler was called by first bus")
	}

	checkNoError(t, secondBus.Dispatch(ctx, testIn{}))
	if !secondCalled {
		t.Fatal("second bus handler was not called")
	}
}

func TestBus_DispatchAsyncReturnsExactlyOneResult(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	bus.Register(&testHandleCommand{})

	result := bus.DispatchAsync(ctx, testIn{})
	err, ok := <-result
	if !ok {
		t.Fatal("async result channel closed before yielding a result")
	}
	checkNoError(t, err)

	if _, ok = <-result; ok {
		t.Fatal("async result channel yielded more than one result")
	}
}

func TestBus_DispatchAllocationBudget(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	bus.Register(&testHandleCommand{})

	var dispatchErr error
	allocations := testing.AllocsPerRun(allocationRuns, func() {
		dispatchErr = bus.Dispatch(ctx, testIn{value: testValueIn})
	})
	checkNoError(t, dispatchErr)

	if allocations != 0 {
		t.Fatalf("Dispatch allocations = %v, want 0", allocations)
	}
}

// Go 1.27.0, median of 5 runs.
// goos: darwin
// goarch: arm64
// cpu: Apple M5 Pro
// Benchmark_Register-12     18833055        62.13 ns/op       344 B/op       3 allocs/op.
func Benchmark_Register(b *testing.B) {
	bus := gobus.New()

	for i := 0; i < b.N; i++ {
		bus.Register(&testHandleCommand{})
	}
}

// Go 1.27.0, median of 5 runs.
// goos: darwin
// goarch: arm64
// cpu: Apple M5 Pro
// Benchmark_Dispatch-12     129305526        9.259 ns/op        0 B/op       0 allocs/op.
func Benchmark_Dispatch(b *testing.B) {
	ctx := context.Background()
	bus := gobus.New()
	bus.Register(&testHandleCommand{})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bus.Dispatch(ctx, testIn{value: testValueIn, index: i})
	}
}

type testHandleCommand struct{}

func (h *testHandleCommand) Execute(_ context.Context, dto testIn) error {
	if dto.err != nil {
		return dto.err
	}

	return nil
}

type testHandleCommand2 struct{}

func (h *testHandleCommand2) Execute(_ context.Context, dto *testIn) error {
	if dto.err != nil {
		return dto.err
	}

	return nil
}

type testTrackingCommandHandler struct {
	called *bool
}

func (h *testTrackingCommandHandler) Execute(_ context.Context, _ testIn) error {
	*h.called = true

	return nil
}
