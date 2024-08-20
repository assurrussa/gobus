package bus_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	bus "github.com/assurrussa/gobus/internal/performance/buscontract"
)

func TestBus_Handle(t *testing.T) {
	ctx := context.Background()
	bus.Register[testIn, testOut](testIn{}, &testHandle{val: "handle"})
	bus.Register[*testInPointer, *testOut](&testInPointer{}, &testHandle2{val: "handle"})

	out, err := bus.Dispatch[testIn, testOut](ctx, testIn{value: "test", index: 1})
	checkNoError(t, err)
	checkEqual(t, "test_handle", out.value)

	outPointer, err := bus.Dispatch[*testInPointer, *testOut](ctx, &testInPointer{value: "test", index: 1})
	checkNoError(t, err)
	checkEqual(t, "test_handle", outPointer.value)

	wg := sync.WaitGroup{}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Register[testIn, testOut](testIn{}, &testHandle{val: "handle"})
		}()
	}

	for i := 0; i < 100; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := bus.Dispatch[testIn, testOut](ctx, testIn{value: "test", index: i})
			checkNoError(t, err)
			checkEqual(t, "test_handle", out.value)
		}()
	}

	for i := 0; i < 100; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := <-bus.DispatchAsync[testIn, testOut](ctx, testIn{value: "test", index: i})
			checkNoError(t, out.Error)
			checkEqual(t, "test_handle", out.Result.value)
		}()
	}

	for i := 100; i < 200; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			errExpect := errors.New("test error")
			out, err := bus.Dispatch[testIn, testOut](ctx, testIn{value: "test", index: i, err: errExpect})
			checkError(t, err, errExpect)
			checkEqual(t, "", out.value)
		}()
	}

	wg.Wait()
}

// goos: linux
// goarch: amd64
// cpu: 11th Gen Intel(R) Core(TM) i7-11700F @ 2.50GHz
// BenchmarkRegister-16             6823029               164.5 ns/op           360 B/op          4 allocs/op
// goos: darwin
// goarch: arm64
// cpu: Apple M1
// BenchmarkRegister-8      5781318               200.1 ns/op           360 B/op          4 allocs/op

func BenchmarkRegister(b *testing.B) {
	for i := 0; i < b.N; i++ {
		bus.Register[testIn, testOut](testIn{}, &testHandle{val: "handle"})
	}
}

// goos: linux
// goarch: amd64
// cpu: 11th Gen Intel(R) Core(TM) i7-11700F @ 2.50GHz
// BenchmarkDispatch-16            34229446                33.52 ns/op           16 B/op          1 allocs/op
// goos: darwin
// goarch: arm64
// cpu: Apple M1
// BenchmarkDispatch-8     23980215                49.13 ns/op           16 B/op          1 allocs/op.
func BenchmarkDispatch(b *testing.B) {
	ctx := context.Background()
	bus.Register[testIn, testOut](testIn{}, &testHandle{val: "handle"})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = bus.Dispatch[testIn, testOut](ctx, testIn{value: "test", index: i})
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
