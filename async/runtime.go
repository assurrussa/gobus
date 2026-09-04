package async

import (
	"context"
	"fmt"
	"reflect"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/assurrussa/gobus"
)

// DefaultQueueName is the reserved name reported for the default queue.
const DefaultQueueName = "default"

// State describes the lifecycle state of a Runtime.
type State string

const (
	// StateNew is the configurable state before Start.
	StateNew State = "new"
	// StateRunning accepts and executes jobs.
	StateRunning State = "running"
	// StateClosing rejects new jobs while accepted jobs finish.
	StateClosing State = "closing"
	// StateClosed has no live workers and cannot be restarted.
	StateClosed State = "closed"
)

// QueueConfig defines the bounded capacity and fixed concurrency of a queue.
// Capacity bounds jobs waiting for a worker; Workers bounds jobs executing in
// parallel. At most Capacity + Workers accepted jobs can wait or execute.
type QueueConfig struct {
	Capacity int
	Workers  int
}

// QueueStats contains individually sampled metrics for one queue.
type QueueStats struct {
	Capacity  int
	Workers   int
	Depth     int
	Active    int64
	Accepted  uint64
	Completed uint64
	Rejected  uint64
}

// Stats contains a best-effort runtime and queue metrics snapshot.
// Values from different fields need not represent the same instant.
type Stats struct {
	State  State
	Queues map[string]QueueStats
}

type routeKind uint8

const (
	commandRoute routeKind = iota
	resultRoute
	eventRoute
)

type routeKey struct {
	kind   routeKind
	input  reflect.Type
	output reflect.Type
}

type queuedJob struct {
	executionContext context.Context //nolint:containedctx // Accepted jobs retain their execution context.

	run     func(context.Context)
	resolve func(error)
}

type queue struct {
	config    QueueConfig
	jobs      chan queuedJob
	active    atomic.Int64
	accepted  atomic.Uint64
	completed atomic.Uint64
	rejected  atomic.Uint64
}

func newQueue(config QueueConfig) *queue {
	return &queue{
		config: config,
		jobs:   make(chan queuedJob, config.Capacity),
	}
}

// Runtime executes gobus operations through bounded in-memory queues.
// A Runtime is single-use and must not be copied.
type Runtime struct {
	bus *gobus.Bus

	mu     sync.Mutex
	state  State
	queues map[string]*queue
	routes map[routeKey]string

	closing chan struct{}
	done    chan struct{}

	forceContext context.Context //nolint:containedctx // Runtime cancellation must reach every active job.
	forceCancel  context.CancelCauseFunc

	admissions sync.WaitGroup
	workers    sync.WaitGroup
}

// New constructs a configurable Runtime with a required default queue.
// It does not start workers.
func New(bus *gobus.Bus, defaultQueue QueueConfig) (*Runtime, error) {
	if bus == nil {
		return nil, ErrNilBus
	}
	if err := validateQueueConfig(defaultQueue); err != nil {
		return nil, err
	}

	forceContext, forceCancel := context.WithCancelCause(context.Background())

	return &Runtime{
		bus:          bus,
		state:        StateNew,
		queues:       map[string]*queue{DefaultQueueName: newQueue(defaultQueue)},
		routes:       make(map[routeKey]string),
		closing:      make(chan struct{}),
		done:         make(chan struct{}),
		forceContext: forceContext,
		forceCancel:  forceCancel,
	}, nil
}

// AddQueue adds a named bounded queue. Configuration is frozen by Start.
func (r *Runtime) AddQueue(name string, config QueueConfig) error {
	if err := validateQueueName(name); err != nil {
		return err
	}
	if err := validateQueueConfig(config); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state != StateNew {
		return ErrConfigFrozen
	}
	if _, ok := r.queues[name]; ok {
		return fmt.Errorf("%w: %s", ErrQueueExists, name)
	}

	r.queues[name] = newQueue(config)
	return nil
}

// RouteCommand routes commands of type Q to a named queue.
func (r *Runtime) RouteCommand[Q gobus.ObjectIn](queueName string) error {
	return r.setRoute(routeKey{kind: commandRoute, input: reflect.TypeFor[Q]()}, queueName)
}

// RouteResult routes the query/result pair Q,T to a named queue.
func (r *Runtime) RouteResult[Q gobus.ObjectIn, T gobus.ObjectOut](queueName string) error {
	return r.setRoute(routeKey{
		kind:   resultRoute,
		input:  reflect.TypeFor[Q](),
		output: reflect.TypeFor[T](),
	}, queueName)
}

// RouteEvent routes events of type E to a named queue.
func (r *Runtime) RouteEvent[E gobus.ObjectIn](queueName string) error {
	return r.setRoute(routeKey{kind: eventRoute, input: reflect.TypeFor[E]()}, queueName)
}

func (r *Runtime) setRoute(key routeKey, queueName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state != StateNew {
		return ErrConfigFrozen
	}
	if _, ok := r.queues[queueName]; !ok {
		return fmt.Errorf("%w: %s", ErrQueueNotFound, queueName)
	}

	r.routes[key] = queueName
	return nil
}

// Start freezes configuration and starts every queue's workers.
func (r *Runtime) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch r.state {
	case StateNew:
		// Continue below.
	case StateRunning:
		return ErrRuntimeStarted
	case StateClosing, StateClosed:
		return ErrRuntimeClosed
	default:
		return fmt.Errorf("async: unknown runtime state %q", r.state)
	}

	for _, configuredQueue := range r.queues {
		for range configuredQueue.config.Workers {
			r.workers.Add(1)
			go r.worker(configuredQueue)
		}
	}
	r.state = StateRunning

	return nil
}

// Shutdown stops admission, drains accepted work, and waits for workers.
// The first call's context controls forced cancellation. Subsequent calls only
// bound their own wait for the same shutdown.
//
// Shutdown must be coordinated outside jobs running on r. A job that calls
// Shutdown on its own Runtime waits for every worker, including itself, and
// blocks until ctx expires if it cannot return first.
func (r *Runtime) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}

	first, alreadyClosed := r.beginShutdown()
	if alreadyClosed {
		return nil
	}
	if first {
		go r.drain()
	}

	select {
	case <-r.done:
		return nil
	default:
	}

	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		select {
		case <-r.done:
			return nil
		default:
		}
		if first {
			r.forceCancel(ErrRuntimeShutdown)
			r.cancelPending()
		}
		return ctx.Err()
	}
}

func (r *Runtime) beginShutdown() (first bool, alreadyClosed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch r.state {
	case StateNew:
		r.state = StateClosed
		close(r.closing)
		close(r.done)
		return false, true
	case StateRunning:
		r.state = StateClosing
		close(r.closing)
		return true, false
	case StateClosing:
		return false, false
	case StateClosed:
		return false, true
	default:
		return false, false
	}
}

func (r *Runtime) drain() {
	r.admissions.Wait()

	r.mu.Lock()
	for _, configuredQueue := range r.queues {
		close(configuredQueue.jobs)
	}
	r.mu.Unlock()

	r.workers.Wait()

	r.mu.Lock()
	r.state = StateClosed
	close(r.done)
	r.mu.Unlock()
}

func (r *Runtime) cancelPending() {
	r.mu.Lock()
	queues := make([]*queue, 0, len(r.queues))
	for _, configuredQueue := range r.queues {
		queues = append(queues, configuredQueue)
	}
	r.mu.Unlock()

	for _, configuredQueue := range queues {
	drainQueue:
		for {
			select {
			case job, ok := <-configuredQueue.jobs:
				if !ok {
					break drainQueue
				}
				job.resolve(ErrRuntimeShutdown)
				configuredQueue.completed.Add(1)
			default:
				break drainQueue
			}
		}
	}
}

func (r *Runtime) worker(configuredQueue *queue) {
	defer r.workers.Done()

	for job := range configuredQueue.jobs {
		r.execute(configuredQueue, job)
	}
}

func (r *Runtime) execute(configuredQueue *queue, job queuedJob) {
	if err := job.executionContext.Err(); err != nil {
		job.resolve(err)
		configuredQueue.completed.Add(1)
		return
	}
	if context.Cause(r.forceContext) != nil {
		job.resolve(ErrRuntimeShutdown)
		configuredQueue.completed.Add(1)
		return
	}

	executionContext, cancel := context.WithCancelCause(job.executionContext)
	stopForcePropagation := context.AfterFunc(r.forceContext, func() {
		cancel(ErrRuntimeShutdown)
	})
	defer func() {
		stopForcePropagation()
		cancel(nil)
	}()

	configuredQueue.active.Add(1)
	defer configuredQueue.active.Add(-1)
	defer configuredQueue.completed.Add(1)

	defer func() {
		if recovered := recover(); recovered != nil {
			job.resolve(&PanicError{Value: recovered, Stack: string(debug.Stack())})
		}
	}()

	job.run(executionContext)
}

// Stats returns a concurrency-safe snapshot of runtime and queue state.
// Because queue depth and counters are individually atomic and update
// concurrently with running workers, the result is a best-effort,
// eventually consistent metrics snapshot rather than a linearizable
// point-in-time state.
func (r *Runtime) Stats() Stats {
	r.mu.Lock()
	defer r.mu.Unlock()

	stats := Stats{
		State:  r.state,
		Queues: make(map[string]QueueStats, len(r.queues)),
	}
	for name, configuredQueue := range r.queues {
		stats.Queues[name] = QueueStats{
			Capacity:  configuredQueue.config.Capacity,
			Workers:   configuredQueue.config.Workers,
			Depth:     len(configuredQueue.jobs),
			Active:    configuredQueue.active.Load(),
			Accepted:  configuredQueue.accepted.Load(),
			Completed: configuredQueue.completed.Load(),
			Rejected:  configuredQueue.rejected.Load(),
		}
	}

	return stats
}

func validateQueueConfig(config QueueConfig) error {
	if config.Capacity <= 0 {
		return fmt.Errorf("%w: capacity must be positive", ErrInvalidQueueConfig)
	}
	if config.Workers <= 0 {
		return fmt.Errorf("%w: workers must be positive", ErrInvalidQueueConfig)
	}
	return nil
}

func validateQueueName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name must not be empty", ErrInvalidQueueConfig)
	}
	if name == DefaultQueueName {
		return fmt.Errorf("%w: name %q is reserved", ErrInvalidQueueConfig, name)
	}
	return nil
}
