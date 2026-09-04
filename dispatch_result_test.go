package gobus_test

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"

	"github.com/assurrussa/gobus"
)

const (
	allocationRuns          = 1000
	maxResultDispatchAllocs = 1
	testValueIn             = "test-in"
	testValueHandle         = "test-handle"
	testFirstHandlerValue   = "first"
	testSecondHandlerValue  = "second"
)

func TestBus_ResultCommandExecutor_ExecuteComplex(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()

	out, err := bus.DispatchResult[testOut](ctx, testIn{value: testValueIn, index: 1})
	checkError(t, err, gobus.ErrHandlerNotFound)
	checkEqual(t, "", out.value)

	outAsyncEnvelope := <-bus.DispatchResultAsync[testOut](ctx, testIn{value: testValueIn, index: 1})
	checkError(t, outAsyncEnvelope.Error, gobus.ErrHandlerNotFound)
	checkEqual(t, "", outAsyncEnvelope.Result.value)

	bus.RegisterResult(&testHandle{val: testValueHandle})
	bus.RegisterResult(&testHandle2{val: testValueHandle})

	out, err = bus.DispatchResult[testOut](ctx, testIn{value: testValueIn, index: 1})
	checkNoError(t, err)
	checkEqual(t, "test-in_test-handle", out.value)

	outPointer, err := bus.DispatchResult[*testOut](ctx, &testIn{value: testValueIn, index: 1})
	checkNoError(t, err)
	checkEqual(t, "test-in_test-handle", outPointer.value)

	wg := sync.WaitGroup{}

	errs := make(chan error, 3000)

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.RegisterResult(&testHandle{val: testValueHandle})
		}()
	}

	for i := 0; i < 1000; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := bus.DispatchResult[testOut](ctx, testIn{value: testValueIn, index: i})
			if err != nil {
				errs <- err
			} else if out.value != "test-in_test-handle" {
				errs <- fmt.Errorf("expected test-in_test-handle, got %s", out.value)
			}
		}()
	}

	for i := 0; i < 1000; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := <-bus.DispatchResultAsync[testOut](ctx, testIn{value: testValueIn, index: i})
			if out.Error != nil {
				errs <- out.Error
			} else if out.Result.value != "test-in_test-handle" {
				errs <- fmt.Errorf("expected test-in_test-handle, got %s", out.Result.value)
			}
		}()
	}

	for i := 1000; i < 2000; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			errExpect := errors.New("test error")
			out, err := bus.DispatchResult[testOut](ctx, testIn{value: testValueIn, index: i, err: errExpect})
			if !errors.Is(err, errExpect) {
				errs <- fmt.Errorf("expected error %s, got %w", errExpect.Error(), err)
			} else if out.value != "" {
				errs <- fmt.Errorf("expected empty value, got %s", out.value)
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent dispatch result error: %v", err)
	}
}

func TestBus_ResultHandlersAreIsolated(t *testing.T) {
	ctx := context.Background()
	firstBus := gobus.New()
	secondBus := gobus.New()
	firstBus.RegisterResult(&testHandle{val: testFirstHandlerValue})
	secondBus.RegisterResult(&testHandle{val: testSecondHandlerValue})

	first, err := firstBus.DispatchResult[testOut](ctx, testIn{value: testValueIn})
	checkNoError(t, err)
	checkEqual(t, "test-in_first", first.value)

	second, err := secondBus.DispatchResult[testOut](ctx, testIn{value: testValueIn})
	checkNoError(t, err)
	checkEqual(t, "test-in_second", second.value)
}

func TestBus_ResultHandlerRegistrationReplacesSameTypePair(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	bus.RegisterResult(&testHandle{val: testFirstHandlerValue})
	bus.RegisterResult(&testHandle{val: testSecondHandlerValue})

	out, err := bus.DispatchResult[testOut](ctx, testIn{value: testValueIn})
	checkNoError(t, err)
	checkEqual(t, "test-in_second", out.value)
}

func TestBus_ResultHandlersWithSameInputAndDifferentOutputsCoexist(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	bus.RegisterResult(&testHandle{val: testValueHandle})
	bus.RegisterResult(&testStringHandle{val: testValueHandle})

	structOut, err := bus.DispatchResult[testOut](ctx, testIn{value: testValueIn})
	checkNoError(t, err)
	checkEqual(t, "test-in_test-handle", structOut.value)

	stringOut, err := bus.DispatchResult[string](ctx, testIn{value: testValueIn})
	checkNoError(t, err)
	checkEqual(t, "test-in_test-handle", stringOut)
}

func TestBus_ResultHandlerRejectsWrongOutputType(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	bus.RegisterResult(&testHandle{val: testValueHandle})

	out, err := bus.DispatchResult[int](ctx, testIn{value: testValueIn})
	checkError(t, err, gobus.ErrHandlerNotFound)
	checkEqual(t, 0, out)
}

func TestBus_ConcurrentResultRegistrationsPreserveDistinctHandlers(t *testing.T) {
	ctx := context.Background()

	for range 1000 {
		bus := gobus.New()
		start := make(chan struct{})
		wg := sync.WaitGroup{}
		wg.Add(2)

		go func() {
			defer wg.Done()
			<-start
			bus.RegisterResult(&testHandle{val: testValueHandle})
		}()

		go func() {
			defer wg.Done()
			<-start
			bus.RegisterResult(&testStringHandle{val: testValueHandle})
		}()

		close(start)
		wg.Wait()

		_, err := bus.DispatchResult[testOut](ctx, testIn{value: testValueIn})
		checkNoError(t, err)
		_, err = bus.DispatchResult[string](ctx, testIn{value: testValueIn})
		checkNoError(t, err)
	}
}

func TestBus_DispatchResultAsyncPreservesPartialResult(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	bus.RegisterResult(&partialResultHandler{})

	envelope := <-bus.DispatchResultAsync[testOut](ctx, testIn{})
	checkEqual(t, "partial", envelope.Result.value)
	if envelope.Error == nil || envelope.Error.Error() != "partial failure" {
		t.Fatalf("expected 'partial failure' error, got %v", envelope.Error)
	}
}

func TestBus_DispatchResultAsyncRecoversPanic(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	panicVal := errors.New("result boom")
	bus.RegisterResult(&panicResultHandler{val: panicVal})

	envelope := <-bus.DispatchResultAsync[testOut](ctx, testIn{})
	if envelope.Error == nil {
		t.Fatal("expected panic error, got nil")
	}
	var panicErr *gobus.PanicError
	if !errors.As(envelope.Error, &panicErr) {
		t.Fatalf("expected *gobus.PanicError, got %T (%v)", envelope.Error, envelope.Error)
	}
	if !errors.Is(envelope.Error, panicVal) {
		t.Fatalf("expected error to unwrap to %v, got %v", panicVal, envelope.Error)
	}
}

func TestBus_DispatchResultAllocationBudget(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	bus.RegisterResult(&testHandle{val: testValueHandle})

	var (
		out         testOut
		dispatchErr error
	)
	allocations := testing.AllocsPerRun(allocationRuns, func() {
		out, dispatchErr = bus.DispatchResult[testOut](ctx, testIn{value: testValueIn})
	})
	checkNoError(t, dispatchErr)
	checkEqual(t, "test-in_test-handle", out.value)

	if allocations > maxResultDispatchAllocs {
		t.Fatalf(
			"DispatchResult allocations = %v, want at most %d",
			allocations,
			maxResultDispatchAllocs,
		)
	}
}

func Benchmark_RegisterResultReplace(b *testing.B) {
	bus := gobus.New()

	for i := 0; i < b.N; i++ {
		bus.RegisterResult(&testHandle{val: testValueHandle})
	}
}

func Benchmark_DispatchResult(b *testing.B) {
	ctx := context.Background()
	bus := gobus.New()
	bus.RegisterResult(&testHandle{val: testValueHandle})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = bus.DispatchResult[testOut](ctx, testIn{value: testValueIn, index: i})
	}
}

type testHandle struct {
	val string
}

func (h *testHandle) Execute(_ context.Context, dto testIn) (testOut, error) {
	if dto.err != nil {
		return testOut{}, dto.err
	}

	return testOut{value: dto.value + "_" + h.val}, nil
}

type testHandle2 struct {
	val string
}

func (h *testHandle2) Execute(_ context.Context, dto *testIn) (*testOut, error) {
	if dto.err != nil {
		return nil, dto.err
	}

	return &testOut{value: dto.value + "_" + h.val}, nil
}

type testStringHandle struct {
	val string
}

func (h *testStringHandle) Execute(_ context.Context, dto testIn) (string, error) {
	return dto.value + "_" + h.val, nil
}

type testIn struct {
	value string
	index int
	err   error
}

type testOut struct {
	value string
}

type partialResultHandler struct{}

func (h *partialResultHandler) Execute(_ context.Context, _ testIn) (testOut, error) {
	return testOut{value: "partial"}, errors.New("partial failure")
}

type panicResultHandler struct {
	val any
}

func (h *panicResultHandler) Execute(_ context.Context, _ testIn) (testOut, error) {
	panic(h.val)
}

func checkError(t *testing.T, err error, targetErr error) {
	t.Helper()

	if err == nil {
		t.Fatalf("check error: %v", err)
	}
	if targetErr != nil && !errors.Is(err, targetErr) {
		t.Fatalf("check error: expected %q, got %q", targetErr, err)
	}
}

func checkNoError(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("check error: %v", err)
	}
}

func checkEqual(t *testing.T, value, expected any) {
	t.Helper()

	if expected != value {
		t.Fatalf("value not equal to expected value: %v != %v", value, expected)
	}
}

func TestBus_ZeroValueIsUsable_Result(t *testing.T) {
	ctx := context.Background()
	var bus gobus.Bus

	// Unregistered result dispatch returns ErrHandlerNotFound.
	_, err := bus.DispatchResult[testOut](ctx, testIn{value: testValueIn})
	checkError(t, err, gobus.ErrHandlerNotFound)

	asyncEnv := <-bus.DispatchResultAsync[testOut](ctx, testIn{value: testValueIn})
	checkError(t, asyncEnv.Error, gobus.ErrHandlerNotFound)

	// Registering on zero-value bus works.
	bus.RegisterResult(&testHandle{val: testValueHandle})

	out, err := bus.DispatchResult[testOut](ctx, testIn{value: testValueIn})
	checkNoError(t, err)
	checkEqual(t, out.value, testValueIn+"_"+testValueHandle)

	asyncEnv = <-bus.DispatchResultAsync[testOut](ctx, testIn{value: testValueIn})
	checkNoError(t, asyncEnv.Error)
	checkEqual(t, asyncEnv.Result.value, testValueIn+"_"+testValueHandle)
}

func TestBus_DispatchResultInvalidRegistryEntry(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	key := gobus.ResultCommandTypeFor[testIn, testOut]()
	bus.InjectInvalidResultCommandHandler(key, "not-a-result-handler")

	_, err := bus.DispatchResult[testOut](ctx, testIn{})
	if !errors.Is(err, gobus.ErrInvalidRegistryEntryForTest) {
		t.Fatalf("DispatchResult error = %v, want ErrInvalidRegistryEntryForTest", err)
	}
}

func TestBus_RegisterResultNilHandlerPanics(t *testing.T) {
	var bus gobus.Bus
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on nil handler, got nil")
		}
	}()
	bus.RegisterResult[testIn, testOut](nil)
}

func TestBus_RegisterResultTypedNilHandlerPanics(t *testing.T) {
	var bus gobus.Bus
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on typed nil handler, got nil")
		}
	}()
	var handler *testHandle
	bus.RegisterResult[testIn, testOut](handler)
}

type goexitResultHandler struct{}

func (h *goexitResultHandler) Execute(_ context.Context, _ testIn) (testOut, error) {
	runtime.Goexit()
	return testOut{}, nil
}

func TestBus_DispatchResultAsyncReportsGoexit(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	bus.RegisterResult(&goexitResultHandler{})

	envelope, ok := <-bus.DispatchResultAsync[testOut](ctx, testIn{})
	if !ok {
		t.Fatal("channel closed without yielding a result")
	}
	if !errors.Is(envelope.Error, gobus.ErrHandlerGoexit) {
		t.Fatalf("expected ErrHandlerGoexit, got %v", envelope.Error)
	}
}
