package async

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/assurrussa/gobus"
)

const shutdownTestTimeout = 3 * time.Second

func TestRuntimeShutdownWaitsForPendingResolution(t *testing.T) {
	rt, releaseWorker := newOccupiedRuntime(t)
	drainWaiting := make(chan struct{})
	rt.beforeWorkersWaitForTest = func() { close(drainWaiting) }

	resolving := make(chan struct{})
	resumeResolution := make(chan struct{})
	releaseResolution := sync.OnceFunc(func() { close(resumeResolution) })
	defer releaseResolution()
	result := make(chan error, 1)
	job := queuedJob{
		executionContext: context.Background(),
		run: func(context.Context) {
			t.Error("pending job executed during forced shutdown")
		},
		resolve: func(err error) {
			close(resolving)
			<-resumeResolution
			result <- err
			close(result)
		},
	}
	if err := rt.enqueue(context.Background(), routeKey{}, job, false); err != nil {
		t.Fatalf("enqueue pending job: %v", err)
	}

	shutdownCtx, cancelShutdown := context.WithCancel(context.Background())
	defer cancelShutdown()
	firstShutdown := make(chan error, 1)
	go func() { firstShutdown <- rt.Shutdown(shutdownCtx) }()

	// drain has already skipped the forced cleanup and is about to wait for workers.
	awaitShutdownSignal(t, drainWaiting, "drain reaching workers.Wait")
	cancelShutdown()
	// cancelPending now owns the last queued job but has not delivered its result.
	awaitShutdownSignal(t, resolving, "pending result resolution")
	releaseWorker()
	rt.workers.Wait()

	waitCtx, cancelWait := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelWait()
	if err := rt.Shutdown(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown before pending result delivery = %v, want DeadlineExceeded", err)
	}
	stats := rt.Stats()
	if stats.State != StateClosing {
		t.Fatalf("state before pending result delivery = %q, want closing", stats.State)
	}
	q := stats.Queues[DefaultQueueName]
	if q.Accepted != 2 || q.Completed != 1 || q.Depth != 0 || q.Active != 0 {
		t.Fatalf("stats while pending result is held = %+v", q)
	}
	select {
	case <-rt.done:
		t.Fatal("runtime done closed before pending result delivery")
	default:
	}

	releaseResolution()
	if err := awaitShutdownError(t, firstShutdown); !errors.Is(err, context.Canceled) {
		t.Fatalf("first Shutdown = %v, want Canceled", err)
	}
	finishRuntimeShutdown(t, rt)
	assertShutdownResult(t, result)
	q = rt.Stats().Queues[DefaultQueueName]
	if q.Accepted != 2 || q.Completed != 2 || q.Depth != 0 || q.Active != 0 {
		t.Fatalf("stats after shutdown = %+v", q)
	}
}

func TestRuntimeForcedShutdownResolvesLateEnqueue(t *testing.T) {
	// Once resumed, enqueue's select can legally choose either the ready send or
	// the closed runtime channel. Require a successful late admission so this test
	// cannot pass by exercising only the rejection branch.
	for range 64 {
		if checkLateEnqueue(t) {
			return
		}
	}
	t.Fatal("no successful late enqueue observed")
}

func checkLateEnqueue(t *testing.T) bool {
	t.Helper()
	rt, releaseWorker := newOccupiedRuntime(t)
	defer releaseWorker()
	entered := make(chan struct{})
	resume := make(chan struct{})
	releaseAdmission := sync.OnceFunc(func() { close(resume) })
	defer releaseAdmission()
	admissionCtx := pausedAdmissionContext{Context: context.Background(), entered: entered, resume: resume}
	type submitOutcome struct {
		result <-chan error
		err    error
	}
	outcomes := make(chan submitOutcome, 1)
	go func() {
		result, err := rt.Submit(admissionCtx, struct{}{}, WithExecutionContext(context.Background()))
		outcomes <- submitOutcome{result: result, err: err}
	}()
	awaitShutdownSignal(t, entered, "admission past the preliminary closing check")

	shutdownCtx, cancelShutdown := context.WithCancel(context.Background())
	cancelShutdown()
	if err := rt.Shutdown(shutdownCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("forced Shutdown = %v, want Canceled", err)
	}
	// The first cleanup is finished. The admission still holds drain at admissions.Wait.
	releaseAdmission()
	var outcome submitOutcome
	select {
	case outcome = <-outcomes:
	case <-time.After(shutdownTestTimeout):
		t.Fatal("timed out waiting for late admission")
	}
	if outcome.err != nil {
		if outcome.result != nil || !errors.Is(outcome.err, ErrRuntimeClosed) {
			t.Fatalf("late Submit = (%v, %v), want (nil, ErrRuntimeClosed)", outcome.result, outcome.err)
		}
		releaseWorker()
		finishRuntimeShutdown(t, rt)
		return false
	}

	// The busy worker is deliberately held until this result arrives. Only the
	// final cancelPending after admissions.Wait can resolve the newly queued job.
	if err := awaitShutdownError(t, outcome.result); !errors.Is(err, ErrRuntimeShutdown) {
		t.Fatalf("late result = %v, want ErrRuntimeShutdown", err)
	}
	if _, ok := <-outcome.result; ok {
		t.Fatal("late result channel did not close after exactly one result")
	}
	if q := rt.Stats().Queues[DefaultQueueName]; q.Accepted != 2 || q.Active != 1 || q.Depth != 0 {
		t.Fatalf("stats before worker release = %+v", q)
	}
	releaseWorker()
	finishRuntimeShutdown(t, rt)
	return true
}

// Done is evaluated in enqueue's blocking select, after its preliminary closing
// check and before the send is selected. Execution uses a separate plain context.
type pausedAdmissionContext struct {
	context.Context //nolint:containedctx // Test context wrapper pauses admission while delegating all other methods.
	entered         chan struct{}
	resume          chan struct{}
}

func (c pausedAdmissionContext) Done() <-chan struct{} {
	close(c.entered)
	<-c.resume
	return c.Context.Done()
}

func newOccupiedRuntime(t *testing.T) (*Runtime, func()) {
	t.Helper()
	rt, err := New(gobus.New(), QueueConfig{Capacity: 1, Workers: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := rt.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	started := make(chan struct{})
	resume := make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	t.Cleanup(func() {
		release()
		finishRuntimeShutdown(t, rt)
	})
	job := queuedJob{
		executionContext: context.Background(),
		run: func(context.Context) {
			close(started)
			<-resume
		},
		resolve: func(err error) { t.Errorf("occupied job resolved without running: %v", err) },
	}
	if err := rt.enqueue(context.Background(), routeKey{}, job, false); err != nil {
		t.Fatalf("enqueue occupied job: %v", err)
	}
	awaitShutdownSignal(t, started, "occupied worker")
	return rt, release
}

func awaitShutdownSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(shutdownTestTimeout):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func awaitShutdownError(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err, ok := <-result:
		if !ok {
			t.Fatal("result channel closed without a result")
		}
		return err
	case <-time.After(shutdownTestTimeout):
		t.Fatal("timed out waiting for shutdown result")
		return nil
	}
}

func assertShutdownResult(t *testing.T, result <-chan error) {
	t.Helper()
	select {
	case err, ok := <-result:
		if !ok || !errors.Is(err, ErrRuntimeShutdown) {
			t.Fatalf("pending result = (%v, %v), want (ErrRuntimeShutdown, true)", err, ok)
		}
	default:
		t.Fatal("Shutdown succeeded before pending result was delivered")
	}
	if _, ok := <-result; ok {
		t.Fatal("pending result channel did not close after exactly one result")
	}
}

func finishRuntimeShutdown(t *testing.T, rt *Runtime) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTestTimeout)
	defer cancel()
	if err := rt.Shutdown(ctx); err != nil {
		t.Fatalf("final Shutdown: %v", err)
	}
	if state := rt.Stats().State; state != StateClosed {
		t.Fatalf("final state = %q, want closed", state)
	}
}
