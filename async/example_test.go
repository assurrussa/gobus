package async_test

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/assurrussa/gobus"
	busasync "github.com/assurrussa/gobus/async"
)

func ExampleRuntime() {
	ctx := context.Background()
	bus := gobus.New()
	bus.Register(exampleCommandHandler{})

	runtime, err := busasync.New(bus, busasync.QueueConfig{Capacity: 16, Workers: 2})
	if err != nil {
		fmt.Printf("new runtime: %v\n", err)
		return
	}
	if err := runtime.Start(); err != nil {
		fmt.Printf("start: %v\n", err)
		return
	}

	result, err := runtime.Submit(ctx, exampleCommand{value: "hello"})
	if err != nil {
		fmt.Printf("submit: %v\n", err)
		return
	}
	_, _ = fmt.Fprintln(os.Stdout, <-result)

	if err := runtime.Shutdown(ctx); err != nil {
		fmt.Printf("shutdown: %v\n", err)
		return
	}
	// Output: <nil>
}

type exampleCommand struct {
	value string
}

type exampleCommandHandler struct{}

func (exampleCommandHandler) Execute(_ context.Context, command exampleCommand) error {
	if command.value == "" {
		return errors.New("value is empty")
	}
	return nil
}
