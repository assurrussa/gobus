package busevent_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/assurrussa/gobus/internal/performance/busevent"
)

const (
	testValueIn     = "test"
	testValueHandle = "handle"
)

func TestBus_Handle(t *testing.T) {
	ctx := context.Background()
	dispatcher := busevent.NewCommandEvent()
	dispatcher.Register(testIn{}, &testHandle{val: testValueHandle})

	var out testOut
	err := dispatcher.Dispatch(ctx, testIn{value: testValueIn, index: 1}, &out)
	checkNoError(t, err)
	checkEqual(t, "test_handle", out.value)
	outEnvelope := <-dispatcher.DispatchAsync(ctx, testIn{value: testValueIn, index: 1})
	checkNoError(t, outEnvelope.Error)
	out, ok := outEnvelope.Result.(testOut)
	checkEqual(t, true, ok)
	checkEqual(t, "test_handle", out.value)

	wg := sync.WaitGroup{}
	errs := make(chan error, 200)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dispatcher.Register(testIn{}, &testHandle{val: testValueHandle})
		}()
	}

	for i := 0; i < 100; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out testOut
			err := dispatcher.Dispatch(ctx, testIn{value: testValueIn, index: i}, &out)
			if err != nil {
				errs <- err
			} else if out.value != "test_handle" {
				errs <- fmt.Errorf("expected test_handle, got %s", out.value)
			}
		}()
	}

	for i := 100; i < 200; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			errExpect := errors.New("test error")
			var out testOut
			err := dispatcher.Dispatch(ctx, testIn{value: testValueIn, index: i, err: errExpect}, &out)
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
// BenchmarkRegister-12     12205840        99.96 ns/op       360 B/op       4 allocs/op.
func BenchmarkRegister(b *testing.B) {
	dispatcher := busevent.NewCommandEvent()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dispatcher.Register(testIn{}, &testHandle{val: testValueHandle})
	}
}

// Go 1.27.0, median of 5 runs.
// goos: darwin
// goarch: arm64
// cpu: Apple M5 Pro
// BenchmarkDispatch-12     27955166        44.73 ns/op        80 B/op       3 allocs/op.
func BenchmarkDispatch(b *testing.B) {
	ctx := context.Background()
	dispatcher := busevent.NewCommandEvent()
	dispatcher.Register(testIn{}, &testHandle{val: testValueHandle})
	var out testOut
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = dispatcher.Dispatch(ctx, testIn{value: testValueIn, index: i}, &out)
	}
}

type testHandle struct {
	val string
}

func (h *testHandle) Execute(_ context.Context, dto any) (any, error) {
	d, ok := dto.(testIn)
	if !ok || d.err != nil {
		return testOut{}, d.err
	}

	return testOut{value: d.value + "_" + h.val}, nil
}

type testIn struct {
	value string
	index int
	err   error
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
