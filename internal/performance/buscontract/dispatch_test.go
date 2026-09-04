package bus_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	bus "github.com/assurrussa/gobus/internal/performance/buscontract"
)

const (
	testValueIn     = "test"
	testValueHandle = "handle"
)

func TestBus_Handle(t *testing.T) {
	ctx := context.Background()
	bus.Register[testIn, testOut](testIn{}, &testHandle{val: testValueHandle})
	bus.Register[*testInPointer, *testOut](&testInPointer{}, &testHandle2{val: testValueHandle})

	out, err := bus.Dispatch[testIn, testOut](ctx, testIn{value: testValueIn, index: 1})
	checkNoError(t, err)
	checkEqual(t, "test_handle", out.value)

	outPointer, err := bus.Dispatch[*testInPointer, *testOut](ctx, &testInPointer{value: testValueIn, index: 1})
	checkNoError(t, err)
	checkEqual(t, "test_handle", outPointer.value)

	wg := sync.WaitGroup{}

	errs := make(chan error, 300)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Register[testIn, testOut](testIn{}, &testHandle{val: testValueHandle})
		}()
	}

	for i := 0; i < 100; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := bus.Dispatch[testIn, testOut](ctx, testIn{value: testValueIn, index: i})
			if err != nil {
				errs <- err
			} else if out.value != "test_handle" {
				errs <- fmt.Errorf("expected test_handle, got %s", out.value)
			}
		}()
	}

	for i := 0; i < 100; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := <-bus.DispatchAsync[testIn, testOut](ctx, testIn{value: testValueIn, index: i})
			if out.Error != nil {
				errs <- out.Error
			} else if out.Result.value != "test_handle" {
				errs <- fmt.Errorf("expected test_handle, got %s", out.Result.value)
			}
		}()
	}

	for i := 100; i < 200; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			errExpect := errors.New("test error")
			out, err := bus.Dispatch[testIn, testOut](ctx, testIn{value: testValueIn, index: i, err: errExpect})
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
		t.Errorf("concurrent dispatch error: %v", err)
	}
}

// Go 1.27.0, median of 5 runs.
// goos: darwin
// goarch: arm64
// cpu: Apple M5 Pro
// BenchmarkRegister-12     18988856        61.69 ns/op       360 B/op       4 allocs/op.

func BenchmarkRegister(b *testing.B) {
	for i := 0; i < b.N; i++ {
		bus.Register[testIn, testOut](testIn{}, &testHandle{val: testValueHandle})
	}
}

// Go 1.27.0, median of 5 runs.
// goos: darwin
// goarch: arm64
// cpu: Apple M5 Pro
// BenchmarkDispatch-12     55050921        20.49 ns/op        16 B/op       1 allocs/op.
func BenchmarkDispatch(b *testing.B) {
	ctx := context.Background()
	bus.Register[testIn, testOut](testIn{}, &testHandle{val: testValueHandle})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = bus.Dispatch[testIn, testOut](ctx, testIn{value: testValueIn, index: i})
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

func (h *testHandle2) Execute(_ context.Context, dto *testInPointer) (*testOut, error) {
	if dto.err != nil {
		return nil, dto.err
	}

	return &testOut{value: dto.value + "_" + h.val}, nil
}

type testIn struct {
	value string
	index int
	err   error
}

func (testIn) Key() string {
	return "test-in"
}

type testInPointer struct {
	value string
	index int
	err   error
}

func (testInPointer) Key() string {
	return "test-in-pointer"
}

type testOut struct {
	value string
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
