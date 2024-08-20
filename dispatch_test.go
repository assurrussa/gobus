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
	gobus.InitCommand()

	err := gobus.Dispatch[testIn](ctx, testIn{value: "test", index: 1})
	checkError(t, err, nil)

	err = <-gobus.DispatchAsync[testIn](ctx, testIn{value: "test", index: 1})
	checkError(t, err, nil)

	gobus.Register[testIn](&testHandleCommand{})
	gobus.Register[*testIn](&testHandleCommand2{})

	err = gobus.Dispatch[testIn](ctx, testIn{value: "test", index: 1})
	checkNoError(t, err)

	err = gobus.Dispatch[*testIn](ctx, &testIn{value: "test", index: 1})
	checkNoError(t, err)

	wg := sync.WaitGroup{}

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gobus.Register[testIn](&testHandleCommand{})
		}()
	}

	for i := 0; i < 1000; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := gobus.Dispatch[testIn](ctx, testIn{value: "test", index: i})
			checkNoError(t, err)
		}()
	}

	for i := 0; i < 1000; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := <-gobus.DispatchAsync[testIn](ctx, testIn{value: "test", index: i})
			checkNoError(t, err)
		}()
	}

	for i := 1000; i < 2000; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			errExpect := errors.New("test error")
			err := gobus.Dispatch[testIn](ctx, testIn{value: "test", index: i, err: errExpect})
			checkError(t, err, errExpect)
		}()
	}

	wg.Wait()
}

// goos: linux
// goarch: amd64
// cpu: 11th Gen Intel(R) Core(TM) i7-11700F @ 2.50GHz
// Benchmark_Register-16      7917388               150.2 ns/op           344 B/op          3 allocs/op
// goos: darwin
// goarch: arm64
// cpu: Apple M1
// Benchmark_Register-8.
func Benchmark_Register(b *testing.B) {
	for i := 0; i < b.N; i++ {
		gobus.Register[testIn](&testHandleCommand{})
	}
}

// goos: linux
// goarch: amd64
// cpu: 11th Gen Intel(R) Core(TM) i7-11700F @ 2.50GHz
// Benchmark_Dispatch-16     85649173                13.85 ns/op            0 B/op          0 allocs/op
// goos: darwin
// goarch: arm64
// cpu: Apple M1
// Benchmark_Dispatch-8.
func Benchmark_Dispatch(b *testing.B) {
	ctx := context.Background()
	gobus.Register[testIn](&testHandleCommand{})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gobus.Dispatch[testIn](ctx, testIn{value: "test", index: i})
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
