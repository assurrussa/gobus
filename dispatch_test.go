package gobus_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
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

	errs := make(chan error, 3000)

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
			if err := bus.Dispatch(ctx, testIn{value: testValueIn, index: i}); err != nil {
				errs <- err
			}
		}()
	}

	for i := 0; i < 1000; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := <-bus.DispatchAsync(ctx, testIn{value: testValueIn, index: i}); err != nil {
				errs <- err
			}
		}()
	}

	for i := 1000; i < 2000; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			errExpect := errors.New("test error")
			err := bus.Dispatch(ctx, testIn{value: testValueIn, index: i, err: errExpect})
			if !errors.Is(err, errExpect) {
				errs <- fmt.Errorf("expected error %s, got %w", errExpect.Error(), err)
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent dispatch error: %v", err)
	}
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

func TestBus_CompileTimeTypeRouting(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	bus.Register(&testHandleCommand{})

	// Registered as testIn, dispatching as any searches for any, not testIn.
	var command any = testIn{value: testValueIn}
	err := bus.Dispatch(ctx, command)
	checkError(t, err, gobus.ErrHandlerNotFound)
}

func TestBus_DispatchAsyncRecoversPanic(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	panicVal := errors.New("boom")
	bus.Register(&testPanicCommand{val: panicVal})

	ch := bus.DispatchAsync(ctx, testPanicIn{})
	err := <-ch
	if err == nil {
		t.Fatal("expected panic error, got nil")
	}
	var panicErr *gobus.PanicError
	if !errors.As(err, &panicErr) {
		t.Fatalf("expected *gobus.PanicError, got %T (%v)", err, err)
	}
	if !errors.Is(err, panicVal) {
		t.Fatalf("expected error to unwrap to %v, got %v", panicVal, err)
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

func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gobus.New()
	}
}

func Benchmark_RegisterReplace(b *testing.B) {
	bus := gobus.New()

	for i := 0; i < b.N; i++ {
		bus.Register(&testHandleCommand{})
	}
}

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

type testPanicIn struct{}

type testPanicCommand struct {
	val any
}

func (h *testPanicCommand) Execute(_ context.Context, _ testPanicIn) error {
	panic(h.val)
}

func TestBus_ZeroValueIsUsable(t *testing.T) {
	ctx := context.Background()
	var bus gobus.Bus

	// Unregistered dispatch returns ErrHandlerNotFound.
	err := bus.Dispatch(ctx, testIn{value: testValueIn})
	checkError(t, err, gobus.ErrHandlerNotFound)

	asyncErr := <-bus.DispatchAsync(ctx, testIn{value: testValueIn})
	checkError(t, asyncErr, gobus.ErrHandlerNotFound)

	// Registering on a zero-value bus works.
	bus.Register(&testHandleCommand{})

	err = bus.Dispatch(ctx, testIn{value: testValueIn})
	checkNoError(t, err)

	asyncErr = <-bus.DispatchAsync(ctx, testIn{value: testValueIn})
	checkNoError(t, asyncErr)
}

func TestBus_DispatchInvalidRegistryEntry(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	key := reflect.TypeFor[testIn]()
	bus.InjectInvalidCommandHandler(key, "not-a-handler")

	err := bus.Dispatch(ctx, testIn{})
	if !errors.Is(err, gobus.ErrInvalidRegistryEntryForTest) {
		t.Fatalf("Dispatch error = %v, want ErrInvalidRegistryEntryForTest", err)
	}
}

func TestBus_NewIsUsable(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()

	err := bus.Dispatch(ctx, testIn{value: testValueIn})
	checkError(t, err, gobus.ErrHandlerNotFound)

	bus.Register(&testHandleCommand{})
	err = bus.Dispatch(ctx, testIn{value: testValueIn})
	checkNoError(t, err)
}

func TestBus_RegisterNilHandlerPanics(t *testing.T) {
	var bus gobus.Bus
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on nil handler, got nil")
		}
	}()
	bus.Register[testIn](nil)
}

func TestBus_RegisterTypedNilHandlerPanics(t *testing.T) {
	var bus gobus.Bus
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on typed nil handler, got nil")
		}
	}()
	var handler *testHandleCommand
	bus.Register[testIn](handler)
}

type testGoexitIn struct{}

type testGoexitCommand struct{}

func (h *testGoexitCommand) Execute(_ context.Context, _ testGoexitIn) error {
	runtime.Goexit()
	return nil
}

func TestBus_DispatchAsyncReportsGoexit(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	bus.Register(&testGoexitCommand{})

	ch := bus.DispatchAsync(ctx, testGoexitIn{})
	err, ok := <-ch
	if !ok {
		t.Fatal("channel closed without yielding a result")
	}
	if !errors.Is(err, gobus.ErrHandlerGoexit) {
		t.Fatalf("expected ErrHandlerGoexit, got %v", err)
	}
	if _, ok := <-ch; ok {
		t.Fatal("channel yielded more than one result")
	}
}
