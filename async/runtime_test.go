package async_test

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/assurrussa/gobus"
	busasync "github.com/assurrussa/gobus/async"
)

const testTimeout = 3 * time.Second

func TestNewAndConfigurationValidation(t *testing.T) {
	tests := []struct {
		name string
		bus  *gobus.Bus
		cfg  busasync.QueueConfig
		want error
	}{
		{name: "nil bus", cfg: validQueueConfig(), want: busasync.ErrNilBus},
		{name: "zero capacity", bus: gobus.New(), cfg: busasync.QueueConfig{Workers: 1}, want: busasync.ErrInvalidQueueConfig},
		{
			name: "negative capacity", bus: gobus.New(),
			cfg: busasync.QueueConfig{Capacity: -1, Workers: 1}, want: busasync.ErrInvalidQueueConfig,
		},
		{name: "zero workers", bus: gobus.New(), cfg: busasync.QueueConfig{Capacity: 1}, want: busasync.ErrInvalidQueueConfig},
		{
			name: "negative workers", bus: gobus.New(),
			cfg: busasync.QueueConfig{Capacity: 1, Workers: -1}, want: busasync.ErrInvalidQueueConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime, err := busasync.New(tt.bus, tt.cfg)
			if runtime != nil {
				t.Fatalf("New() runtime = %v, want nil", runtime)
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("New() error = %v, want errors.Is(%v)", err, tt.want)
			}
		})
	}
}

func TestRuntimeConfigurationAndLifecycle(t *testing.T) {
	runtime := newRuntime(t, gobus.New(), validQueueConfig())
	if got := runtime.Stats().State; got != busasync.StateNew {
		t.Fatalf("initial state = %q, want %q", got, busasync.StateNew)
	}

	invalidConfigs := []struct {
		name string
		cfg  busasync.QueueConfig
	}{
		{name: "", cfg: validQueueConfig()},
		{name: busasync.DefaultQueueName, cfg: validQueueConfig()},
		{name: "invalid-capacity", cfg: busasync.QueueConfig{Workers: 1}},
		{name: "invalid-workers", cfg: busasync.QueueConfig{Capacity: 1}},
	}
	for _, tt := range invalidConfigs {
		if err := runtime.AddQueue(tt.name, tt.cfg); !errors.Is(err, busasync.ErrInvalidQueueConfig) {
			t.Fatalf("AddQueue(%q) error = %v, want ErrInvalidQueueConfig", tt.name, err)
		}
	}

	if err := runtime.AddQueue("io", busasync.QueueConfig{Capacity: 3, Workers: 2}); err != nil {
		t.Fatalf("AddQueue(io) error = %v", err)
	}
	if err := runtime.AddQueue("io", validQueueConfig()); !errors.Is(err, busasync.ErrQueueExists) {
		t.Fatalf("duplicate AddQueue() error = %v, want ErrQueueExists", err)
	}
	if err := runtime.RouteCommand[testCommand]("missing"); !errors.Is(err, busasync.ErrQueueNotFound) {
		t.Fatalf("RouteCommand(missing) error = %v, want ErrQueueNotFound", err)
	}
	if err := runtime.RouteCommand[testCommand]("io"); err != nil {
		t.Fatalf("RouteCommand(io) error = %v", err)
	}
	if err := runtime.RouteCommand[testCommand](busasync.DefaultQueueName); err != nil {
		t.Fatalf("RouteCommand(default) replacement error = %v", err)
	}
	if err := runtime.RouteResult[testQuery, testOutput]("io"); err != nil {
		t.Fatalf("RouteResult(io) error = %v", err)
	}
	if err := runtime.RouteEvent[testEvent]("io"); err != nil {
		t.Fatalf("RouteEvent(io) error = %v", err)
	}

	if err := runtime.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got := runtime.Stats().State; got != busasync.StateRunning {
		t.Fatalf("running state = %q, want %q", got, busasync.StateRunning)
	}
	if err := runtime.Start(); !errors.Is(err, busasync.ErrRuntimeStarted) {
		t.Fatalf("second Start() error = %v, want ErrRuntimeStarted", err)
	}
	if err := runtime.AddQueue("late", validQueueConfig()); !errors.Is(err, busasync.ErrConfigFrozen) {
		t.Fatalf("AddQueue after Start error = %v, want ErrConfigFrozen", err)
	}
	if err := runtime.RouteEvent[testEvent](busasync.DefaultQueueName); !errors.Is(err, busasync.ErrConfigFrozen) {
		t.Fatalf("RouteEvent after Start error = %v, want ErrConfigFrozen", err)
	}

	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
	if got := runtime.Stats().State; got != busasync.StateClosed {
		t.Fatalf("closed state = %q, want %q", got, busasync.StateClosed)
	}
	if err := runtime.Start(); !errors.Is(err, busasync.ErrRuntimeClosed) {
		t.Fatalf("Start after Shutdown error = %v, want ErrRuntimeClosed", err)
	}
	result, err := runtime.Submit(context.Background(), testCommand{})
	if result != nil || !errors.Is(err, busasync.ErrRuntimeClosed) {
		t.Fatalf("Submit after Shutdown = (%v, %v), want (nil, ErrRuntimeClosed)", result, err)
	}
}

func TestRuntimeShutdownBeforeStartClosesRuntime(t *testing.T) {
	runtime := newRuntime(t, gobus.New(), validQueueConfig())

	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown before Start error = %v", err)
	}
	if got := runtime.Stats().State; got != busasync.StateClosed {
		t.Fatalf("state = %q, want %q", got, busasync.StateClosed)
	}
	if err := runtime.Start(); !errors.Is(err, busasync.ErrRuntimeClosed) {
		t.Fatalf("Start after pre-start Shutdown error = %v, want ErrRuntimeClosed", err)
	}
}

func TestRuntimeSubmitBeforeStart(t *testing.T) {
	runtime := newRuntime(t, gobus.New(), validQueueConfig())

	result, err := runtime.Submit(context.Background(), testCommand{})
	if result != nil || !errors.Is(err, busasync.ErrRuntimeNotStarted) {
		t.Fatalf("Submit before Start = (%v, %v), want (nil, ErrRuntimeNotStarted)", result, err)
	}
	stats := runtime.Stats().Queues[busasync.DefaultQueueName]
	if stats.Rejected != 1 {
		t.Fatalf("rejected = %d, want 1", stats.Rejected)
	}
}

func TestRuntimeExecutesCommandsQueriesAndEventsOnNamedQueue(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	bus.Register(commandHandler[testCommand]{execute: func(_ context.Context, command testCommand) error {
		if command.id != 7 {
			return fmt.Errorf("command id = %d", command.id)
		}
		return nil
	}})
	bus.RegisterResult(resultHandler[testQuery, testOutput]{execute: func(_ context.Context, query testQuery) (testOutput, error) {
		return testOutput(query), nil
	}})
	eventErr := errors.New("event subscriber failed")
	var eventCalls atomic.Int64
	bus.Subscribe(eventHandler[testEvent]{execute: func(_ context.Context, event testEvent) error {
		eventCalls.Add(int64(event.id))
		return eventErr
	}})
	bus.Subscribe(eventHandler[testEvent]{execute: func(_ context.Context, _ testEvent) error {
		eventCalls.Add(1)
		return nil
	}})

	runtime := newRuntime(t, bus, validQueueConfig())
	if err := runtime.AddQueue("work", busasync.QueueConfig{Capacity: 8, Workers: 2}); err != nil {
		t.Fatalf("AddQueue() error = %v", err)
	}
	if err := runtime.RouteCommand[testCommand]("work"); err != nil {
		t.Fatalf("RouteCommand() error = %v", err)
	}
	if err := runtime.RouteResult[testQuery, testOutput]("work"); err != nil {
		t.Fatalf("RouteResult() error = %v", err)
	}
	if err := runtime.RouteEvent[testEvent]("work"); err != nil {
		t.Fatalf("RouteEvent() error = %v", err)
	}
	startRuntime(t, runtime)

	commandResult, err := runtime.Submit(ctx, testCommand{id: 7})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if err := receiveError(t, commandResult); err != nil {
		t.Fatalf("command execution error = %v", err)
	}
	assertClosed(t, commandResult)

	queryResult, err := runtime.SubmitResult[testOutput](ctx, testQuery{id: 9})
	if err != nil {
		t.Fatalf("SubmitResult() error = %v", err)
	}
	envelope := receiveEnvelope(t, queryResult)
	if envelope.Error != nil || envelope.Result.id != 9 {
		t.Fatalf("query envelope = %+v, want result id 9", envelope)
	}
	assertEnvelopeClosed(t, queryResult)

	tryQueryResult, err := runtime.TrySubmitResult[testOutput](ctx, testQuery{id: 10})
	if err != nil {
		t.Fatalf("TrySubmitResult() error = %v", err)
	}
	tryEnvelope := receiveEnvelope(t, tryQueryResult)
	if tryEnvelope.Error != nil || tryEnvelope.Result.id != 10 {
		t.Fatalf("try query envelope = %+v, want result id 10", tryEnvelope)
	}

	eventResult, err := runtime.SubmitEvent(ctx, testEvent{id: 3})
	if err != nil {
		t.Fatalf("SubmitEvent() error = %v", err)
	}
	if err := receiveError(t, eventResult); !errors.Is(err, eventErr) {
		t.Fatalf("event execution error = %v, want eventErr", err)
	}
	if got := eventCalls.Load(); got != 4 {
		t.Fatalf("event subscriber total = %d, want 4", got)
	}

	eventWithoutSubscribers, err := runtime.TrySubmitEvent(ctx, otherEvent{})
	if err != nil {
		t.Fatalf("TrySubmitEvent(no subscribers) error = %v", err)
	}
	if err := receiveError(t, eventWithoutSubscribers); err != nil {
		t.Fatalf("event without subscribers execution error = %v", err)
	}

	missingResult, err := runtime.Submit(ctx, missingCommand{})
	if err != nil {
		t.Fatalf("Submit(missing) admission error = %v", err)
	}
	if err := receiveError(t, missingResult); !errors.Is(err, gobus.ErrHandlerNotFound) {
		t.Fatalf("missing command execution error = %v, want ErrHandlerNotFound", err)
	}

	waitFor(t, func() bool {
		return runtime.Stats().Queues["work"].Completed == 4
	}, "named queue completion")
	stats := runtime.Stats()
	work := stats.Queues["work"]
	if work.Capacity != 8 || work.Workers != 2 || work.Accepted != 4 || work.Completed != 4 {
		t.Fatalf("work stats = %+v, want capacity=8 workers=2 accepted=4 completed=4", work)
	}
	if got := stats.Queues[busasync.DefaultQueueName].Accepted; got != 2 {
		t.Fatalf("default accepted = %d, want 2", got)
	}
}

func TestRuntimeTrySubmitRejectsFullQueue(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	bus.Register(commandHandler[testCommand]{execute: func(_ context.Context, _ testCommand) error {
		started <- struct{}{}
		<-release
		return nil
	}})

	runtime := newRuntime(t, bus, busasync.QueueConfig{Capacity: 1, Workers: 1})
	startRuntime(t, runtime)
	first, err := runtime.Submit(ctx, testCommand{id: 1})
	if err != nil {
		t.Fatalf("first Submit() error = %v", err)
	}
	receiveSignal(t, started, "first handler start")
	second, err := runtime.Submit(ctx, testCommand{id: 2})
	if err != nil {
		t.Fatalf("second Submit() error = %v", err)
	}

	third, err := runtime.TrySubmit(ctx, testCommand{id: 3})
	if third != nil || !errors.Is(err, busasync.ErrQueueFull) {
		t.Fatalf("TrySubmit(full) = (%v, %v), want (nil, ErrQueueFull)", third, err)
	}

	cancelledContext, cancel := context.WithCancel(ctx)
	cancel()
	cancelled, err := runtime.TrySubmit(cancelledContext, testCommand{id: 4})
	if cancelled != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("TrySubmit(cancelled) = (%v, %v), want (nil, context.Canceled)", cancelled, err)
	}

	close(release)
	if err := receiveError(t, first); err != nil {
		t.Fatalf("first result = %v", err)
	}
	if err := receiveError(t, second); err != nil {
		t.Fatalf("second result = %v", err)
	}
	waitFor(t, func() bool {
		return runtime.Stats().Queues[busasync.DefaultQueueName].Completed == 2
	}, "full queue completion")
	stats := runtime.Stats().Queues[busasync.DefaultQueueName]
	if stats.Accepted != 2 || stats.Rejected != 2 {
		t.Fatalf("queue stats = %+v, want accepted=2 rejected=2", stats)
	}
}

func TestRuntimeCancellationBeforeExecutionSkipsHandler(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var secondCalls atomic.Int64
	bus.Register(commandHandler[testCommand]{execute: func(_ context.Context, command testCommand) error {
		if command.id == 1 {
			started <- struct{}{}
			<-release
			return nil
		}
		secondCalls.Add(1)
		return nil
	}})

	runtime := newRuntime(t, bus, busasync.QueueConfig{Capacity: 2, Workers: 1})
	startRuntime(t, runtime)
	first, err := runtime.Submit(ctx, testCommand{id: 1})
	if err != nil {
		t.Fatalf("first Submit() error = %v", err)
	}
	receiveSignal(t, started, "first handler start")

	jobContext, cancel := context.WithCancel(ctx)
	second, err := runtime.Submit(jobContext, testCommand{id: 2})
	if err != nil {
		t.Fatalf("second Submit() error = %v", err)
	}
	cancel()
	close(release)

	if err := receiveError(t, first); err != nil {
		t.Fatalf("first result = %v", err)
	}
	if err := receiveError(t, second); !errors.Is(err, context.Canceled) {
		t.Fatalf("second result = %v, want context.Canceled", err)
	}
	if got := secondCalls.Load(); got != 0 {
		t.Fatalf("cancelled handler calls = %d, want 0", got)
	}
}

func TestRuntimeExecutionContextSurvivesAdmissionCancellation(t *testing.T) {
	bus := gobus.New()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	executed := make(chan executionContextObservation, 1)
	bus.Register(commandHandler[testCommand]{execute: func(ctx context.Context, command testCommand) error {
		switch command.id {
		case 1:
			started <- struct{}{}
			<-release
		case 2:
			executed <- executionContextObservation{
				err:   ctx.Err(),
				value: ctx.Value(detachedContextKey{}),
			}
		}
		return nil
	}})

	runtime := newRuntime(t, bus, busasync.QueueConfig{Capacity: 2, Workers: 1})
	startRuntime(t, runtime)
	first, err := runtime.Submit(context.Background(), testCommand{id: 1})
	if err != nil {
		t.Fatalf("first Submit() error = %v", err)
	}
	receiveSignal(t, started, "first handler start")

	requestContext, cancelRequest := context.WithCancel(
		context.WithValue(context.Background(), detachedContextKey{}, "request-value"),
	)
	detachedContext := context.WithoutCancel(requestContext)
	second, err := runtime.TrySubmit(
		requestContext,
		testCommand{id: 2},
		busasync.WithExecutionContext(detachedContext),
	)
	if err != nil {
		t.Fatalf("detached TrySubmit() error = %v", err)
	}
	cancelRequest()
	close(release)

	if err := receiveError(t, first); err != nil {
		t.Fatalf("first result = %v", err)
	}
	var observation executionContextObservation
	select {
	case observation = <-executed:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for detached execution observation")
	}
	if observation.err != nil {
		t.Fatalf("detached execution context error = %v, want nil", observation.err)
	}
	if observation.value != "request-value" {
		t.Fatalf("detached execution context value = %v, want request-value", observation.value)
	}
	if err := receiveError(t, second); err != nil {
		t.Fatalf("detached result = %v", err)
	}
}

func TestRuntimeCancelledExecutionContextSkipsHandler(t *testing.T) {
	bus := gobus.New()
	var calls atomic.Int64
	bus.Register(commandHandler[testCommand]{execute: func(_ context.Context, _ testCommand) error {
		calls.Add(1)
		return nil
	}})
	runtime := newRuntime(t, bus, validQueueConfig())
	startRuntime(t, runtime)

	executionContext, cancelExecution := context.WithCancel(context.Background())
	cancelExecution()
	result, err := runtime.Submit(
		context.Background(),
		testCommand{},
		busasync.WithExecutionContext(executionContext),
	)
	if err != nil {
		t.Fatalf("Submit() admission error = %v", err)
	}
	if err := receiveError(t, result); !errors.Is(err, context.Canceled) {
		t.Fatalf("execution result = %v, want context.Canceled", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("handler calls = %d, want 0", got)
	}
}

func TestRuntimeExecutionContextOptionAppliesToAllJobKinds(t *testing.T) {
	bus := gobus.New()
	blockStarted := make(chan struct{}, 1)
	release := make(chan struct{})
	observed := make(chan string, 3)
	bus.Register(commandHandler[blockingCommand]{execute: func(_ context.Context, _ blockingCommand) error {
		blockStarted <- struct{}{}
		<-release
		return nil
	}})
	bus.Register(commandHandler[testCommand]{execute: func(ctx context.Context, _ testCommand) error {
		observed <- "command:" + executionContextValue(ctx)
		return nil
	}})
	bus.RegisterResult(resultHandler[testQuery, testOutput]{execute: func(ctx context.Context, query testQuery) (testOutput, error) {
		observed <- "result:" + executionContextValue(ctx)
		return testOutput(query), nil
	}})
	bus.Subscribe(eventHandler[testEvent]{execute: func(ctx context.Context, _ testEvent) error {
		observed <- "event:" + executionContextValue(ctx)
		return nil
	}})

	runtime := newRuntime(t, bus, busasync.QueueConfig{Capacity: 3, Workers: 1})
	startRuntime(t, runtime)
	blocker, err := runtime.Submit(context.Background(), blockingCommand{})
	if err != nil {
		t.Fatalf("blocking Submit() error = %v", err)
	}
	receiveSignal(t, blockStarted, "blocking handler start")

	commandAdmission, cancelCommandAdmission := context.WithCancel(context.Background())
	commandResult, err := runtime.Submit(
		commandAdmission,
		testCommand{id: 1},
		busasync.WithExecutionContext(context.WithValue(context.Background(), detachedContextKey{}, "command")),
	)
	if err != nil {
		t.Fatalf("command Submit() error = %v", err)
	}
	resultAdmission, cancelResultAdmission := context.WithCancel(context.Background())
	queryResult, err := runtime.TrySubmitResult[testOutput](
		resultAdmission,
		testQuery{id: 2},
		busasync.WithExecutionContext(context.WithValue(context.Background(), detachedContextKey{}, "result")),
	)
	if err != nil {
		t.Fatalf("query TrySubmitResult() error = %v", err)
	}
	eventAdmission, cancelEventAdmission := context.WithCancel(context.Background())
	eventResult, err := runtime.SubmitEvent(
		eventAdmission,
		testEvent{id: 3},
		busasync.WithExecutionContext(context.WithValue(context.Background(), detachedContextKey{}, "event")),
	)
	if err != nil {
		t.Fatalf("event SubmitEvent() error = %v", err)
	}

	cancelCommandAdmission()
	cancelResultAdmission()
	cancelEventAdmission()
	close(release)
	if err := receiveError(t, blocker); err != nil {
		t.Fatalf("blocking result = %v", err)
	}
	if err := receiveError(t, commandResult); err != nil {
		t.Fatalf("command result = %v", err)
	}
	queryEnvelope := receiveEnvelope(t, queryResult)
	if queryEnvelope.Error != nil || queryEnvelope.Result.id != 2 {
		t.Fatalf("query result = %+v, want id 2", queryEnvelope)
	}
	if err := receiveError(t, eventResult); err != nil {
		t.Fatalf("event result = %v", err)
	}

	wantObservations := map[string]bool{
		"command:command": true,
		"result:result":   true,
		"event:event":     true,
	}
	for range len(wantObservations) {
		select {
		case observation := <-observed:
			if !wantObservations[observation] {
				t.Fatalf("unexpected execution context observation %q", observation)
			}
			delete(wantObservations, observation)
		case <-time.After(testTimeout):
			t.Fatalf("timed out waiting for execution contexts: %v", wantObservations)
		}
	}
}

func TestRuntimeRejectsInvalidSubmitOptions(t *testing.T) {
	runtime := newRuntime(t, gobus.New(), validQueueConfig())
	startRuntime(t, runtime)
	nilExecutionContext := busasync.WithExecutionContext(nil) //nolint:staticcheck // Verify defensive public API validation.
	tests := []struct {
		name   string
		option busasync.SubmitOption
	}{
		{name: "zero option", option: busasync.SubmitOption{}},
		{name: "nil execution context", option: nilExecutionContext},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := runtime.Submit(context.Background(), testCommand{}, test.option)
			if result != nil || !errors.Is(err, busasync.ErrInvalidSubmitOption) {
				t.Fatalf("Submit() = (%v, %v), want (nil, ErrInvalidSubmitOption)", result, err)
			}
		})
	}
}

func TestRuntimeExecutionContextOptionOrder(t *testing.T) {
	type orderContextKey struct{}
	bus := gobus.New()
	observed := make(chan string, 1)
	bus.Register(commandHandler[testCommand]{execute: func(ctx context.Context, _ testCommand) error {
		val, _ := ctx.Value(orderContextKey{}).(string)
		observed <- val
		return nil
	}})
	runtime := newRuntime(t, bus, validQueueConfig())
	startRuntime(t, runtime)

	t.Run("last valid option wins", func(t *testing.T) {
		ctx1 := context.WithValue(context.Background(), orderContextKey{}, "first")
		ctx2 := context.WithValue(context.Background(), orderContextKey{}, "second")
		result, err := runtime.Submit(
			context.Background(),
			testCommand{},
			busasync.WithExecutionContext(ctx1),
			busasync.WithExecutionContext(ctx2),
		)
		if err != nil {
			t.Fatalf("Submit() error = %v", err)
		}
		if err := receiveError(t, result); err != nil {
			t.Fatalf("execution error = %v", err)
		}
		select {
		case val := <-observed:
			if val != "second" {
				t.Fatalf("observed context value = %q, want %q", val, "second")
			}
		case <-time.After(testTimeout):
			t.Fatal("timed out waiting for execution")
		}
	})

	t.Run("early nil overridden by last valid option", func(t *testing.T) {
		ctx2 := context.WithValue(context.Background(), orderContextKey{}, "valid")
		nilOption := busasync.WithExecutionContext(nil) //nolint:staticcheck // Verify option ordering.
		result, err := runtime.Submit(
			context.Background(),
			testCommand{},
			nilOption,
			busasync.WithExecutionContext(ctx2),
		)
		if err != nil {
			t.Fatalf("Submit() error = %v", err)
		}
		if err := receiveError(t, result); err != nil {
			t.Fatalf("execution error = %v", err)
		}
		select {
		case val := <-observed:
			if val != "valid" {
				t.Fatalf("observed context value = %q, want %q", val, "valid")
			}
		case <-time.After(testTimeout):
			t.Fatal("timed out waiting for execution")
		}
	})

	t.Run("early valid option overridden by nil option", func(t *testing.T) {
		ctx1 := context.WithValue(context.Background(), orderContextKey{}, "first")
		nilOption := busasync.WithExecutionContext(nil) //nolint:staticcheck // Verify option ordering.
		result, err := runtime.Submit(
			context.Background(),
			testCommand{},
			busasync.WithExecutionContext(ctx1),
			nilOption,
		)
		if result != nil || !errors.Is(err, busasync.ErrInvalidSubmitOption) {
			t.Fatalf("Submit() = (%v, %v), want (nil, ErrInvalidSubmitOption)", result, err)
		}
	})
}

func TestRuntimeRejectsNilAdmissionContext(t *testing.T) {
	runtime := newRuntime(t, gobus.New(), validQueueConfig())
	startRuntime(t, runtime)

	var nilCtx context.Context

	if _, err := runtime.Submit(nilCtx, testCommand{}); !errors.Is(err, busasync.ErrNilContext) {
		t.Fatalf("Submit(nil) error = %v, want ErrNilContext", err)
	}
	if _, err := runtime.TrySubmit(nilCtx, testCommand{}); !errors.Is(err, busasync.ErrNilContext) {
		t.Fatalf("TrySubmit(nil) error = %v, want ErrNilContext", err)
	}
	if _, err := runtime.SubmitResult[testOutput](nilCtx, testQuery{}); !errors.Is(err, busasync.ErrNilContext) {
		t.Fatalf("SubmitResult(nil) error = %v, want ErrNilContext", err)
	}
	if _, err := runtime.TrySubmitResult[testOutput](nilCtx, testQuery{}); !errors.Is(err, busasync.ErrNilContext) {
		t.Fatalf("TrySubmitResult(nil) error = %v, want ErrNilContext", err)
	}
	if _, err := runtime.SubmitEvent(nilCtx, testEvent{}); !errors.Is(err, busasync.ErrNilContext) {
		t.Fatalf("SubmitEvent(nil) error = %v, want ErrNilContext", err)
	}
	if _, err := runtime.TrySubmitEvent(nilCtx, testEvent{}); !errors.Is(err, busasync.ErrNilContext) {
		t.Fatalf("TrySubmitEvent(nil) error = %v, want ErrNilContext", err)
	}
	if err := runtime.Shutdown(nilCtx); !errors.Is(err, busasync.ErrNilContext) {
		t.Fatalf("Shutdown(nil) error = %v, want ErrNilContext", err)
	}
}

func TestRuntime_SubmitResultPreservesPartialResult(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	partialErr := errors.New("partial error")
	bus.RegisterResult(resultHandler[testQuery, testOutput]{
		execute: func(_ context.Context, query testQuery) (testOutput, error) {
			return testOutput(query), partialErr
		},
	})
	runtime := newRuntime(t, bus, validQueueConfig())
	startRuntime(t, runtime)

	// SubmitResult
	queryResult, err := runtime.SubmitResult[testOutput](ctx, testQuery{id: 42})
	if err != nil {
		t.Fatalf("SubmitResult() error = %v", err)
	}
	envelope := receiveEnvelope(t, queryResult)
	if envelope.Result.id != 42 {
		t.Fatalf("SubmitResult result id = %d, want 42", envelope.Result.id)
	}
	if !errors.Is(envelope.Error, partialErr) {
		t.Fatalf("SubmitResult error = %v, want %v", envelope.Error, partialErr)
	}

	// TrySubmitResult
	tryQueryResult, err := runtime.TrySubmitResult[testOutput](ctx, testQuery{id: 43})
	if err != nil {
		t.Fatalf("TrySubmitResult() error = %v", err)
	}
	tryEnvelope := receiveEnvelope(t, tryQueryResult)
	if tryEnvelope.Result.id != 43 {
		t.Fatalf("TrySubmitResult result id = %d, want 43", tryEnvelope.Result.id)
	}
	if !errors.Is(tryEnvelope.Error, partialErr) {
		t.Fatalf("TrySubmitResult error = %v, want %v", tryEnvelope.Error, partialErr)
	}
}

func TestRuntimeRecoversPanicAndKeepsWorkerAlive(t *testing.T) {
	panicValue := errors.New("panic value")
	bus := gobus.New()
	bus.Register(commandHandler[testCommand]{execute: func(_ context.Context, command testCommand) error {
		if command.id == 1 {
			panic(panicValue)
		}
		return nil
	}})
	runtime := newRuntime(t, bus, busasync.QueueConfig{Capacity: 2, Workers: 1})
	startRuntime(t, runtime)

	first, err := runtime.Submit(context.Background(), testCommand{id: 1})
	if err != nil {
		t.Fatalf("first Submit() error = %v", err)
	}
	panicErr := receiveError(t, first)
	var recovered *busasync.PanicError
	if !errors.As(panicErr, &recovered) {
		t.Fatalf("panic result = %v, want *PanicError", panicErr)
	}
	if !strings.Contains(panicErr.Error(), "panic value") {
		t.Fatalf("panic error text = %q, want recovered value", panicErr.Error())
	}
	if !errors.Is(panicErr, panicValue) {
		t.Fatalf("panic result = %v, want errors.Is(panicValue)", panicErr)
	}
	if !strings.Contains(recovered.Stack, "TestRuntimeRecoversPanicAndKeepsWorkerAlive") {
		t.Fatalf("panic stack does not contain test handler: %q", recovered.Stack)
	}

	second, err := runtime.Submit(context.Background(), testCommand{id: 2})
	if err != nil {
		t.Fatalf("second Submit() error = %v", err)
	}
	if err := receiveError(t, second); err != nil {
		t.Fatalf("second result = %v, want nil", err)
	}
}

func TestRuntimeRespectsWorkerLimit(t *testing.T) {
	const workers = 3
	bus := gobus.New()
	release := make(chan struct{})
	started := make(chan struct{}, workers)
	var active atomic.Int64
	var maximum atomic.Int64
	bus.Register(commandHandler[testCommand]{execute: func(_ context.Context, _ testCommand) error {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		started <- struct{}{}
		<-release
		return nil
	}})

	runtime := newRuntime(t, bus, busasync.QueueConfig{Capacity: 6, Workers: workers})
	startRuntime(t, runtime)
	results := make([]<-chan error, 6)
	for i := range results {
		var err error
		results[i], err = runtime.Submit(context.Background(), testCommand{id: i})
		if err != nil {
			t.Fatalf("Submit(%d) error = %v", i, err)
		}
	}
	for range workers {
		receiveSignal(t, started, "worker start")
	}
	if got := maximum.Load(); got != workers {
		t.Fatalf("maximum active = %d, want %d", got, workers)
	}
	stats := runtime.Stats().Queues[busasync.DefaultQueueName]
	if stats.Active != workers || stats.Depth != 3 {
		t.Fatalf("active/depth = %d/%d, want %d/3", stats.Active, stats.Depth, workers)
	}
	close(release)
	for i, result := range results {
		if err := receiveError(t, result); err != nil {
			t.Fatalf("result %d = %v", i, err)
		}
	}
}

func TestRuntimeWorkersOnePreservesOrder(t *testing.T) {
	bus := gobus.New()
	var mu sync.Mutex
	order := make([]int, 0, 5)
	bus.Register(commandHandler[testCommand]{execute: func(_ context.Context, command testCommand) error {
		mu.Lock()
		order = append(order, command.id)
		mu.Unlock()
		return nil
	}})
	runtime := newRuntime(t, bus, busasync.QueueConfig{Capacity: 5, Workers: 1})
	startRuntime(t, runtime)

	results := make([]<-chan error, 5)
	for i := range results {
		var err error
		results[i], err = runtime.Submit(context.Background(), testCommand{id: i})
		if err != nil {
			t.Fatalf("Submit(%d) error = %v", i, err)
		}
	}
	for _, result := range results {
		if err := receiveError(t, result); err != nil {
			t.Fatalf("ordered result = %v", err)
		}
	}
	for i, got := range order {
		if got != i {
			t.Fatalf("execution order = %v, want [0 1 2 3 4]", order)
		}
	}
}

func TestRuntimeResolvesHandlersAtExecution(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	blockStarted := make(chan struct{}, 1)
	release := make(chan struct{})
	bus.Register(commandHandler[blockingCommand]{execute: func(_ context.Context, _ blockingCommand) error {
		blockStarted <- struct{}{}
		<-release
		return nil
	}})
	var oldCalls atomic.Int64
	var newCalls atomic.Int64
	bus.Register(commandHandler[testCommand]{execute: func(_ context.Context, _ testCommand) error {
		oldCalls.Add(1)
		return nil
	}})
	var eventCalls atomic.Int64
	bus.Subscribe(eventHandler[testEvent]{execute: func(_ context.Context, _ testEvent) error {
		eventCalls.Add(1)
		return nil
	}})

	runtime := newRuntime(t, bus, busasync.QueueConfig{Capacity: 3, Workers: 1})
	startRuntime(t, runtime)
	blocking, err := runtime.Submit(ctx, blockingCommand{})
	if err != nil {
		t.Fatalf("blocking Submit() error = %v", err)
	}
	receiveSignal(t, blockStarted, "blocking handler start")
	commandResult, err := runtime.Submit(ctx, testCommand{})
	if err != nil {
		t.Fatalf("command Submit() error = %v", err)
	}
	eventResult, err := runtime.SubmitEvent(ctx, testEvent{})
	if err != nil {
		t.Fatalf("event SubmitEvent() error = %v", err)
	}

	bus.Register(commandHandler[testCommand]{execute: func(_ context.Context, _ testCommand) error {
		newCalls.Add(1)
		return nil
	}})
	bus.Subscribe(eventHandler[testEvent]{execute: func(_ context.Context, _ testEvent) error {
		eventCalls.Add(10)
		return nil
	}})
	close(release)

	if err := receiveError(t, blocking); err != nil {
		t.Fatalf("blocking result = %v", err)
	}
	if err := receiveError(t, commandResult); err != nil {
		t.Fatalf("command result = %v", err)
	}
	if err := receiveError(t, eventResult); err != nil {
		t.Fatalf("event result = %v", err)
	}
	if oldCalls.Load() != 0 || newCalls.Load() != 1 {
		t.Fatalf("old/new handler calls = %d/%d, want 0/1", oldCalls.Load(), newCalls.Load())
	}
	if got := eventCalls.Load(); got != 11 {
		t.Fatalf("event calls = %d, want 11", got)
	}
}

func TestRuntimeGracefulShutdownDrainsAndRejectsNewWork(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	bus.Register(commandHandler[testCommand]{execute: func(_ context.Context, command testCommand) error {
		if command.id == 1 {
			started <- struct{}{}
			<-release
		}
		return nil
	}})
	runtime := newRuntime(t, bus, busasync.QueueConfig{Capacity: 3, Workers: 1})
	startRuntime(t, runtime)
	first, err := runtime.Submit(ctx, testCommand{id: 1})
	if err != nil {
		t.Fatalf("first Submit() error = %v", err)
	}
	receiveSignal(t, started, "shutdown blocker start")
	second, err := runtime.Submit(ctx, testCommand{id: 2})
	if err != nil {
		t.Fatalf("second Submit() error = %v", err)
	}

	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- runtime.Shutdown(context.Background())
	}()
	waitFor(t, func() bool { return runtime.Stats().State == busasync.StateClosing }, "runtime closing")
	late, err := runtime.Submit(ctx, testCommand{id: 3})
	if late != nil || !errors.Is(err, busasync.ErrRuntimeClosed) {
		t.Fatalf("late Submit() = (%v, %v), want (nil, ErrRuntimeClosed)", late, err)
	}

	close(release)
	if err := receiveError(t, first); err != nil {
		t.Fatalf("first result = %v", err)
	}
	if err := receiveError(t, second); err != nil {
		t.Fatalf("second result = %v", err)
	}
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatalf("Shutdown result = %v", err)
	}
	if got := runtime.Stats().State; got != busasync.StateClosed {
		t.Fatalf("final state = %q, want closed", got)
	}
}

func TestRuntimeShutdownFromOwnHandlerWaitsForContext(t *testing.T) {
	bus := gobus.New()
	entered := make(chan struct{}, 1)
	shutdownResult := make(chan error, 1)
	shutdownContext, cancelShutdown := context.WithCancel(context.Background())
	defer cancelShutdown()

	var runtime *busasync.Runtime
	bus.Register(commandHandler[testCommand]{execute: func(_ context.Context, _ testCommand) error {
		entered <- struct{}{}
		shutdownResult <- runtime.Shutdown(shutdownContext)
		return nil
	}})
	runtime = newRuntime(t, bus, validQueueConfig())
	startRuntime(t, runtime)

	executionResult, err := runtime.Submit(context.Background(), testCommand{})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	receiveSignal(t, entered, "handler Shutdown call")
	waitFor(t, func() bool { return runtime.Stats().State == busasync.StateClosing }, "runtime closing from handler")
	select {
	case err := <-shutdownResult:
		t.Fatalf("handler Shutdown returned before context cancellation: %v", err)
	default:
	}

	cancelShutdown()
	if err := receiveError(t, shutdownResult); !errors.Is(err, context.Canceled) {
		t.Fatalf("handler Shutdown error = %v, want context.Canceled", err)
	}
	if err := receiveError(t, executionResult); err != nil {
		t.Fatalf("handler execution result = %v, want nil", err)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("follow-up Shutdown() error = %v", err)
	}
}

func TestRuntimeShutdownDeadlineCancelsPendingAndExecutionContext(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	bus.Register(commandHandler[testCommand]{execute: func(executionContext context.Context, command testCommand) error {
		if command.id == 1 {
			started <- executionContext
			<-release
		}
		return nil
	}})
	runtime := newRuntime(t, bus, busasync.QueueConfig{Capacity: 2, Workers: 1})
	startRuntime(t, runtime)
	requestContext, cancelRequest := context.WithCancel(ctx)
	detachedContext := context.WithoutCancel(requestContext)
	first, err := runtime.Submit(ctx, testCommand{id: 1}, busasync.WithExecutionContext(detachedContext))
	if err != nil {
		t.Fatalf("first Submit() error = %v", err)
	}
	executionContext := receiveContext(t, started, "running job context")
	cancelRequest()
	second, err := runtime.Submit(ctx, testCommand{id: 2})
	if err != nil {
		t.Fatalf("second Submit() error = %v", err)
	}

	shutdownContext, cancelShutdown := context.WithCancel(ctx)
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- runtime.Shutdown(shutdownContext)
	}()
	waitFor(t, func() bool { return runtime.Stats().State == busasync.StateClosing }, "runtime closing before forced shutdown")
	cancelShutdown()
	if err := receiveError(t, shutdownResult); !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown result = %v, want context.Canceled", err)
	}
	if err := receiveError(t, second); !errors.Is(err, busasync.ErrRuntimeShutdown) {
		t.Fatalf("pending result = %v, want ErrRuntimeShutdown", err)
	}
	select {
	case <-executionContext.Done():
		if !errors.Is(context.Cause(executionContext), busasync.ErrRuntimeShutdown) {
			t.Fatalf("execution context cause = %v, want ErrRuntimeShutdown", context.Cause(executionContext))
		}
	case <-time.After(testTimeout):
		t.Fatal("running execution context was not cancelled")
	}

	close(release)
	if err := receiveError(t, first); err != nil {
		t.Fatalf("running handler result = %v, want actual nil result", err)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("follow-up Shutdown() error = %v", err)
	}
}

func TestBlockedSubmitWakesWhenShutdownStarts(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	bus.Register(commandHandler[testCommand]{execute: func(_ context.Context, _ testCommand) error {
		started <- struct{}{}
		<-release
		return nil
	}})
	runtime := newRuntime(t, bus, busasync.QueueConfig{Capacity: 1, Workers: 1})
	startRuntime(t, runtime)
	first, err := runtime.Submit(ctx, testCommand{id: 1})
	if err != nil {
		t.Fatalf("first Submit() error = %v", err)
	}
	receiveSignal(t, started, "blocked-submit handler start")
	second, err := runtime.Submit(ctx, testCommand{id: 2})
	if err != nil {
		t.Fatalf("second Submit() error = %v", err)
	}

	blockedResult := make(chan error, 1)
	go func() {
		result, submitErr := runtime.Submit(ctx, testCommand{id: 3})
		if result != nil {
			blockedResult <- errors.New("blocked Submit returned a result channel")
			return
		}
		blockedResult <- submitErr
	}()
	waitFor(t, func() bool {
		return runtime.Stats().Queues[busasync.DefaultQueueName].Depth == 1
	}, "full queue before shutdown")

	shutdownContext, cancelShutdown := context.WithCancel(ctx)
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- runtime.Shutdown(shutdownContext)
	}()
	if err := receiveError(t, blockedResult); !errors.Is(err, busasync.ErrRuntimeClosed) {
		t.Fatalf("blocked Submit error = %v, want ErrRuntimeClosed", err)
	}
	cancelShutdown()
	if err := receiveError(t, shutdownResult); !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown result = %v, want context.Canceled", err)
	}

	close(release)
	if err := receiveError(t, first); err != nil {
		t.Fatalf("first result = %v", err)
	}
	secondErr := receiveError(t, second)
	if secondErr != nil && !errors.Is(secondErr, busasync.ErrRuntimeShutdown) {
		t.Fatalf("second result = %v, want nil or ErrRuntimeShutdown", secondErr)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("final Shutdown() error = %v", err)
	}
}

func TestRuntimeResultChannelsDoNotBlockShutdownWhenUnread(t *testing.T) {
	bus := gobus.New()
	bus.Register(commandHandler[testCommand]{execute: func(_ context.Context, _ testCommand) error { return nil }})
	runtime := newRuntime(t, bus, busasync.QueueConfig{Capacity: 8, Workers: 2})
	startRuntime(t, runtime)

	for i := range 8 {
		if _, err := runtime.Submit(context.Background(), testCommand{id: i}); err != nil {
			t.Fatalf("Submit(%d) error = %v", i, err)
		}
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func BenchmarkRuntimeSubmit(b *testing.B) {
	bus := gobus.New()
	bus.Register(commandHandler[testCommand]{execute: func(_ context.Context, _ testCommand) error { return nil }})
	runtime := newRuntime(b, bus, busasync.QueueConfig{Capacity: 64, Workers: 1})
	startRuntime(b, runtime)
	b.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			b.Fatalf("Shutdown() error = %v", err)
		}
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := runtime.Submit(context.Background(), testCommand{id: i})
		if err != nil {
			b.Fatalf("Submit() error = %v", err)
		}
		if err := <-result; err != nil {
			b.Fatalf("execution error = %v", err)
		}
	}
}

func TestRuntimeForcedShutdownDuringAdmission(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()
	workerStarted := make(chan struct{}, 1)
	workerRelease := make(chan struct{})

	bus.Register(commandHandler[testCommand]{
		execute: func(_ context.Context, cmd testCommand) error {
			if cmd.id == 1 {
				workerStarted <- struct{}{}
				<-workerRelease
			}
			return nil
		},
	})

	rt := newRuntime(t, bus, busasync.QueueConfig{Capacity: 1, Workers: 1})
	startRuntime(t, rt)

	firstResult, err := rt.Submit(ctx, testCommand{id: 1})
	if err != nil {
		t.Fatalf("first Submit() error = %v", err)
	}
	receiveSignal(t, workerStarted, "worker 1 occupied")

	admissionEntered := make(chan struct{})
	resumeAdmission := make(chan struct{})
	rt.SetAfterBeginAdmissionForTest(func() {
		close(admissionEntered)
		<-resumeAdmission
	})

	type submitOutcome struct {
		ch  <-chan error
		err error
	}
	outcomeChan := make(chan submitOutcome, 1)

	go func() {
		ch, submitErr := rt.TrySubmit(ctx, testCommand{id: 2})
		outcomeChan <- submitOutcome{ch: ch, err: submitErr}
	}()

	receiveSignal(t, admissionEntered, "admission entered")

	shutdownCtx, cancelShutdown := context.WithCancel(ctx)
	cancelShutdown()
	shutdownErr := rt.Shutdown(shutdownCtx)
	if !errors.Is(shutdownErr, context.Canceled) {
		t.Fatalf("Shutdown error = %v, want context.Canceled", shutdownErr)
	}

	close(resumeAdmission)

	var outcome submitOutcome
	select {
	case outcome = <-outcomeChan:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for TrySubmit to return")
	}

	if outcome.ch != nil || !errors.Is(outcome.err, busasync.ErrRuntimeClosed) {
		t.Fatalf("TrySubmit = (%v, %v), want (nil, ErrRuntimeClosed)", outcome.ch, outcome.err)
	}

	stats := rt.Stats()
	if stats.Queues[busasync.DefaultQueueName].Depth != 0 {
		t.Fatalf("queue depth = %d, want 0", stats.Queues[busasync.DefaultQueueName].Depth)
	}

	close(workerRelease)
	if err := receiveError(t, firstResult); err != nil {
		t.Fatalf("first job error = %v", err)
	}
	if err := rt.Shutdown(context.Background()); err != nil {
		t.Fatalf("final Shutdown error = %v", err)
	}
}

func TestRuntimeWorkerGoexitRecovery(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()

	bus.Register(commandHandler[testCommand]{
		execute: func(_ context.Context, cmd testCommand) error {
			if cmd.id == 1 {
				runtime.Goexit()
			}
			return nil
		},
	})

	rt := newRuntime(t, bus, busasync.QueueConfig{Capacity: 2, Workers: 1})
	startRuntime(t, rt)

	first, err := rt.Submit(ctx, testCommand{id: 1})
	if err != nil {
		t.Fatalf("first Submit() error = %v", err)
	}
	second, err := rt.Submit(ctx, testCommand{id: 2})
	if err != nil {
		t.Fatalf("second Submit() error = %v", err)
	}

	firstErr := receiveError(t, first)
	if !errors.Is(firstErr, busasync.ErrHandlerGoexit) {
		t.Fatalf("first job error = %v, want ErrHandlerGoexit", firstErr)
	}

	secondErr := receiveError(t, second)
	if secondErr != nil {
		t.Fatalf("second job error = %v, want nil", secondErr)
	}

	if err := rt.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown error = %v", err)
	}

	stats := rt.Stats()
	qStats := stats.Queues[busasync.DefaultQueueName]
	if qStats.Depth != 0 {
		t.Fatalf("queue depth = %d, want 0", qStats.Depth)
	}
	if qStats.Completed != 2 {
		t.Fatalf("completed = %d, want 2", qStats.Completed)
	}
}

func TestRuntimeWorkerGoexitResultRecovery(t *testing.T) {
	ctx := context.Background()
	bus := gobus.New()

	bus.RegisterResult(resultHandler[testQuery, testOutput]{
		execute: func(_ context.Context, q testQuery) (testOutput, error) {
			if q.id == 1 {
				runtime.Goexit()
			}
			return testOutput(q), nil
		},
	})

	rt := newRuntime(t, bus, busasync.QueueConfig{Capacity: 2, Workers: 1})
	startRuntime(t, rt)

	first, err := rt.SubmitResult[testOutput](ctx, testQuery{id: 1})
	if err != nil {
		t.Fatalf("first SubmitResult error = %v", err)
	}
	second, err := rt.SubmitResult[testOutput](ctx, testQuery{id: 2})
	if err != nil {
		t.Fatalf("second SubmitResult error = %v", err)
	}

	var firstEnvelope gobus.Envelope[testOutput]
	select {
	case firstEnvelope = <-first:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for first result")
	}
	if !errors.Is(firstEnvelope.Error, busasync.ErrHandlerGoexit) {
		t.Fatalf("first query error = %v, want ErrHandlerGoexit", firstEnvelope.Error)
	}

	var secondEnvelope gobus.Envelope[testOutput]
	select {
	case secondEnvelope = <-second:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for second result")
	}
	if secondEnvelope.Error != nil || secondEnvelope.Result.id != 2 {
		t.Fatalf("second query result = %+v, want id 2 and nil error", secondEnvelope)
	}

	if err := rt.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown error = %v", err)
	}
}

func validQueueConfig() busasync.QueueConfig {
	return busasync.QueueConfig{Capacity: 4, Workers: 1}
}

func newRuntime(tb testing.TB, bus *gobus.Bus, config busasync.QueueConfig) *busasync.Runtime {
	tb.Helper()
	runtime, err := busasync.New(bus, config)
	if err != nil {
		tb.Fatalf("New() error = %v", err)
	}
	return runtime
}

func startRuntime(tb testing.TB, runtime *busasync.Runtime) {
	tb.Helper()
	if err := runtime.Start(); err != nil {
		tb.Fatalf("Start() error = %v", err)
	}
	tb.Cleanup(func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()
		if err := runtime.Shutdown(shutdownContext); err != nil {
			tb.Errorf("cleanup Shutdown() error = %v", err)
		}
	})
}

func receiveError(tb testing.TB, result <-chan error) error {
	tb.Helper()
	select {
	case err, ok := <-result:
		if !ok {
			tb.Fatal("result channel closed before yielding a value")
		}
		return err
	case <-time.After(testTimeout):
		tb.Fatal("timed out waiting for result")
		return nil
	}
}

func receiveEnvelope[T any](tb testing.TB, result <-chan gobus.Envelope[T]) gobus.Envelope[T] {
	tb.Helper()
	select {
	case envelope, ok := <-result:
		if !ok {
			tb.Fatal("result channel closed before yielding an envelope")
		}
		return envelope
	case <-time.After(testTimeout):
		tb.Fatal("timed out waiting for envelope")
		return gobus.Envelope[T]{}
	}
}

func assertClosed(tb testing.TB, result <-chan error) {
	tb.Helper()
	select {
	case _, ok := <-result:
		if ok {
			tb.Fatal("result channel yielded more than one value")
		}
	case <-time.After(testTimeout):
		tb.Fatal("result channel did not close")
	}
}

func assertEnvelopeClosed[T any](tb testing.TB, result <-chan gobus.Envelope[T]) {
	tb.Helper()
	select {
	case _, ok := <-result:
		if ok {
			tb.Fatal("result channel yielded more than one envelope")
		}
	case <-time.After(testTimeout):
		tb.Fatal("result channel did not close")
	}
}

func receiveSignal(tb testing.TB, signal <-chan struct{}, description string) {
	tb.Helper()
	select {
	case <-signal:
	case <-time.After(testTimeout):
		tb.Fatalf("timed out waiting for %s", description)
	}
}

func receiveContext(tb testing.TB, result <-chan context.Context, description string) context.Context {
	tb.Helper()
	select {
	case ctx := <-result:
		return ctx
	case <-time.After(testTimeout):
		tb.Fatalf("timed out waiting for %s", description)
		return context.Background()
	}
}

func waitFor(tb testing.TB, condition func() bool, description string) {
	tb.Helper()
	deadline := time.Now().Add(testTimeout)
	for !condition() {
		if time.Now().After(deadline) {
			tb.Fatalf("timed out waiting for %s", description)
		}
		runtime.Gosched()
	}
}

type commandHandler[Q any] struct {
	execute func(context.Context, Q) error
}

func (h commandHandler[Q]) Execute(ctx context.Context, command Q) error {
	return h.execute(ctx, command)
}

type resultHandler[Q, T any] struct {
	execute func(context.Context, Q) (T, error)
}

func (h resultHandler[Q, T]) Execute(ctx context.Context, query Q) (T, error) {
	return h.execute(ctx, query)
}

type eventHandler[E any] struct {
	execute func(context.Context, E) error
}

func (h eventHandler[E]) Execute(ctx context.Context, event E) error {
	return h.execute(ctx, event)
}

type testCommand struct {
	id int
}

type missingCommand struct{}

type blockingCommand struct{}

type testQuery struct {
	id int
}

type testOutput struct {
	id int
}

type testEvent struct {
	id int
}

type otherEvent struct{}

type detachedContextKey struct{}

type executionContextObservation struct {
	err   error
	value any
}

func executionContextValue(ctx context.Context) string {
	value, _ := ctx.Value(detachedContextKey{}).(string)
	return value
}
