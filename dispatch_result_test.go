package gobus_test

import (
	"context"
	"errors"
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
			checkNoError(t, err)
			checkEqual(t, "test-in_test-handle", out.value)
		}()
	}

	for i := 0; i < 1000; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := <-bus.DispatchResultAsync[testOut](ctx, testIn{value: testValueIn, index: i})
			checkNoError(t, out.Error)
			checkEqual(t, "test-in_test-handle", out.Result.value)
		}()
	}

	for i := 1000; i < 2000; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			errExpect := errors.New("test error")
			out, err := bus.DispatchResult[testOut](ctx, testIn{value: testValueIn, index: i, err: errExpect})
			checkError(t, err, errExpect)
			checkEqual(t, "", out.value)
		}()
	}

	wg.Wait()
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

// Go 1.27.0, median of 5 runs.
// goos: darwin
// goarch: arm64
// cpu: Apple M5 Pro
// Benchmark_RegisterResult-12     16375870        75.08 ns/op       360 B/op       4 allocs/op.
func Benchmark_RegisterResult(b *testing.B) {
	bus := gobus.New()

	for i := 0; i < b.N; i++ {
		bus.RegisterResult(&testHandle{val: testValueHandle})
	}
}

// Go 1.27.0, median of 5 runs.
// goos: darwin
// goarch: arm64
// cpu: Apple M5 Pro
// Benchmark_DispatchResult-12     53863790        22.34 ns/op        24 B/op       1 allocs/op.
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
