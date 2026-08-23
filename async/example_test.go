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

	runtime, _ := busasync.New(bus, busasync.QueueConfig{Capacity: 16, Workers: 2})
	_ = runtime.Start()

	result, _ := runtime.Submit(ctx, exampleCommand{value: "hello"})
	_, _ = fmt.Fprintln(os.Stdout, <-result)

	_ = runtime.Shutdown(ctx)
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
