package busevent_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/assurrussa/gobus/internal/performance/busevent"
)

func TestBus_Handle(t *testing.T) {
	ctx := context.Background()
	dispatcher := busevent.NewCommandEvent()
	dispatcher.Register(testIn{}, &testHandle{val: "handle"})

	var out testOut
	err := dispatcher.Dispatch(ctx, testIn{value: "test", index: 1}, &out)
	checkNoError(t, err)
	checkEqual(t, "test_handle", out.value)
	outEnvelope := <-dispatcher.DispatchAsync(ctx, testIn{value: "test", index: 1})
	checkNoError(t, outEnvelope.Error)
	out = outEnvelope.Result.(testOut)
	checkEqual(t, "test_handle", out.value)

	wg := sync.WaitGroup{}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dispatcher.Register(testIn{}, &testHandle{val: "handle"})
		}()
	}

	for i := 0; i < 100; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out testOut
			err := dispatcher.Dispatch(ctx, testIn{value: "test", index: i}, &out)
			checkNoError(t, err)
			checkEqual(t, "test_handle", out.value)
		}()
	}

	for i := 100; i < 200; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			errExpect := errors.New("test error")
			var out testOut
			err := dispatcher.Dispatch(ctx, testIn{value: "test", index: i, err: errExpect}, &out)
			checkError(t, err, errExpect)
			checkEqual(t, "", out.value)
		}()
	}

	wg.Wait()
}

// goos: linux
// goarch: amd64
// cpu: 11th Gen Intel(R) Core(TM) i7-11700F @ 2.50GHz
// BenchmarkRegister-16             5573889               208.6 ns/op           360 B/op          4 allocs/op
// goos: darwin
// goarch: arm64
// cpu: Apple M1
// BenchmarkRegister-8      4592404               252.5 ns/op           360 B/op          4 allocs/op.
func BenchmarkRegister(b *testing.B) {
	dispatcher := busevent.NewCommandEvent()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dispatcher.Register(testIn{}, &testHandle{val: "handle"})
	}
}

// goos: linux
// goarch: amd64
// cpu: 11th Gen Intel(R) Core(TM) i7-11700F @ 2.50GHz
// BenchmarkDispatch-16            11001295               106.4 ns/op            80 B/op          3 allocs/op
// goos: darwin
// goarch: arm64
// cpu: Apple M1
// BenchmarkDispatch-8      8173801               146.1 ns/op            80 B/op          3 allocs/op.
func BenchmarkDispatch(b *testing.B) {
	ctx := context.Background()
	dispatcher := busevent.NewCommandEvent()
	dispatcher.Register(testIn{}, &testHandle{val: "handle"})
	var out testOut
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = dispatcher.Dispatch(ctx, testIn{value: "test", index: i}, &out)
	}
}

type testHandle struct {
	val string
}

func (h *testHandle) Execute(_ context.Context, dto any) (any, error) {
	d := dto.(testIn)
	if d.err != nil {
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
