package gobus_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/assurrussa/gobus"
)

func TestBus_ResultCommandExecutor_ExecuteComplex(t *testing.T) {
	ctx := context.Background()
	gobus.InitResultCommand()

	out, err := gobus.DispatchResult[testIn, testOut](ctx, testIn{value: "test", index: 1})
	checkError(t, err, nil)
	checkEqual(t, "", out.value)

	outAsyncEnvelope := <-gobus.DispatchResultAsync[testIn, testOut](ctx, testIn{value: "test", index: 1})
	checkError(t, outAsyncEnvelope.Error, nil)
	checkEqual(t, "", outAsyncEnvelope.Result.value)

	gobus.RegisterResult[testIn, testOut](&testHandle{val: "handle"})
	gobus.RegisterResult[*testIn, *testOut](&testHandle2{val: "handle"})

	out, err = gobus.DispatchResult[testIn, testOut](ctx, testIn{value: "test", index: 1})
	checkNoError(t, err)
	checkEqual(t, "test_handle", out.value)

	outPointer, err := gobus.DispatchResult[*testIn, *testOut](ctx, &testIn{value: "test", index: 1})
	checkNoError(t, err)
	checkEqual(t, "test_handle", outPointer.value)

	wg := sync.WaitGroup{}

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gobus.RegisterResult[testIn, testOut](&testHandle{val: "handle"})
		}()
	}

	for i := 0; i < 1000; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := gobus.DispatchResult[testIn, testOut](ctx, testIn{value: "test", index: i})
			checkNoError(t, err)
			checkEqual(t, "test_handle", out.value)
		}()
	}

	for i := 0; i < 1000; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := <-gobus.DispatchResultAsync[testIn, testOut](ctx, testIn{value: "test", index: i})
			checkNoError(t, out.Error)
			checkEqual(t, "test_handle", out.Result.value)
		}()
	}

	for i := 1000; i < 2000; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			errExpect := errors.New("test error")
			out, err := gobus.DispatchResult[testIn, testOut](ctx, testIn{value: "test", index: i, err: errExpect})
			checkError(t, err, errExpect)
			checkEqual(t, "", out.value)
		}()
	}

	wg.Wait()
}

// goos: linux
// goarch: amd64
// cpu: 11th Gen Intel(R) Core(TM) i7-11700F @ 2.50GHz
// Benchmark_RegisterResult-16             6620785               176.6 ns/op           360 B/op          4 allocs/op
// goos: darwin
// goarch: arm64
// cpu: Apple M1
// Benchmark_RegisterResult-8      5569047               208.8 ns/op           360 B/op          4 allocs/op.
func Benchmark_RegisterResult(b *testing.B) {
	for i := 0; i < b.N; i++ {
		gobus.RegisterResult[testIn, testOut](&testHandle{val: "handle"})
	}
}

// goos: linux
// goarch: amd64
// cpu: 11th Gen Intel(R) Core(TM) i7-11700F @ 2.50GHz
// Benchmark_DispatchResult-16            29745181                39.07 ns/op           16 B/op          1 allocs/op
// goos: darwin
// goarch: arm64
// cpu: Apple M1
// Benchmark_DispatchResult-8     20793022                56.82 ns/op           16 B/op          1 allocs/op.
func Benchmark_DispatchResult(b *testing.B) {
	ctx := context.Background()
	gobus.RegisterResult[testIn, testOut](&testHandle{val: "handle"})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gobus.DispatchResult[testIn, testOut](ctx, testIn{value: "test", index: i})
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
