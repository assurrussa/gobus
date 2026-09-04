package async

import (
	"context"
	"reflect"

	"github.com/assurrussa/gobus"
)

// Submit enqueues a command, waiting for capacity or admissionContext
// cancellation. Without WithExecutionContext, admissionContext also controls
// handler execution.
func (r *Runtime) Submit[Q gobus.ObjectIn](
	admissionContext context.Context,
	command Q,
	options ...SubmitOption,
) (<-chan error, error) {
	return r.submitCommand(admissionContext, command, false, options)
}

// TrySubmit checks admissionContext and attempts to enqueue a command without
// waiting for capacity. Without WithExecutionContext, admissionContext also
// controls handler execution.
func (r *Runtime) TrySubmit[Q gobus.ObjectIn](
	admissionContext context.Context,
	command Q,
	options ...SubmitOption,
) (<-chan error, error) {
	return r.submitCommand(admissionContext, command, true, options)
}

func (r *Runtime) submitCommand[Q gobus.ObjectIn](
	admissionContext context.Context,
	command Q,
	try bool,
	options []SubmitOption,
) (<-chan error, error) {
	if admissionContext == nil {
		return nil, ErrNilContext
	}

	executionContext, err := executionContextFor(admissionContext, options)
	if err != nil {
		return nil, err
	}

	result := make(chan error, 1)
	job := queuedJob{
		executionContext: executionContext,
		run: func(executionContext context.Context) {
			result <- r.bus.Dispatch(executionContext, command)
			close(result)
		},
		resolve: func(err error) {
			result <- err
			close(result)
		},
	}

	err = r.enqueue(admissionContext, routeKey{kind: commandRoute, input: reflect.TypeFor[Q]()}, job, try)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SubmitResult enqueues a query, waiting for capacity or admissionContext
// cancellation. Without WithExecutionContext, admissionContext also controls
// handler execution.
func (r *Runtime) SubmitResult[T gobus.ObjectOut, Q gobus.ObjectIn](
	admissionContext context.Context,
	query Q,
	options ...SubmitOption,
) (<-chan gobus.Envelope[T], error) {
	return r.submitResult[T](admissionContext, query, false, options)
}

// TrySubmitResult checks admissionContext and attempts to enqueue a query
// without waiting for capacity. Without WithExecutionContext,
// admissionContext also controls handler execution.
func (r *Runtime) TrySubmitResult[T gobus.ObjectOut, Q gobus.ObjectIn](
	admissionContext context.Context,
	query Q,
	options ...SubmitOption,
) (<-chan gobus.Envelope[T], error) {
	return r.submitResult[T](admissionContext, query, true, options)
}

func (r *Runtime) submitResult[T gobus.ObjectOut, Q gobus.ObjectIn](
	admissionContext context.Context,
	query Q,
	try bool,
	options []SubmitOption,
) (<-chan gobus.Envelope[T], error) {
	if admissionContext == nil {
		return nil, ErrNilContext
	}

	executionContext, err := executionContextFor(admissionContext, options)
	if err != nil {
		return nil, err
	}

	result := make(chan gobus.Envelope[T], 1)
	job := queuedJob{
		executionContext: executionContext,
		run: func(executionContext context.Context) {
			out, err := r.bus.DispatchResult[T](executionContext, query)
			result <- gobus.Envelope[T]{
				Result: out,
				Error:  err,
			}
			close(result)
		},
		resolve: func(err error) {
			result <- gobus.Envelope[T]{Error: err}
			close(result)
		},
	}

	err = r.enqueue(admissionContext, routeKey{
		kind:   resultRoute,
		input:  reflect.TypeFor[Q](),
		output: reflect.TypeFor[T](),
	}, job, try)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SubmitEvent enqueues a complete event publication, waiting for capacity or
// admissionContext cancellation. Without WithExecutionContext,
// admissionContext also controls handler execution.
func (r *Runtime) SubmitEvent[E gobus.ObjectIn](
	admissionContext context.Context,
	event E,
	options ...SubmitOption,
) (<-chan error, error) {
	return r.submitEvent(admissionContext, event, false, options)
}

// TrySubmitEvent checks admissionContext and attempts to enqueue a complete
// event publication without waiting for capacity. Without
// WithExecutionContext, admissionContext also controls handler execution.
func (r *Runtime) TrySubmitEvent[E gobus.ObjectIn](
	admissionContext context.Context,
	event E,
	options ...SubmitOption,
) (<-chan error, error) {
	return r.submitEvent(admissionContext, event, true, options)
}

func (r *Runtime) submitEvent[E gobus.ObjectIn](
	admissionContext context.Context,
	event E,
	try bool,
	options []SubmitOption,
) (<-chan error, error) {
	if admissionContext == nil {
		return nil, ErrNilContext
	}

	executionContext, err := executionContextFor(admissionContext, options)
	if err != nil {
		return nil, err
	}

	result := make(chan error, 1)
	job := queuedJob{
		executionContext: executionContext,
		run: func(executionContext context.Context) {
			result <- r.bus.Publish(executionContext, event)
			close(result)
		},
		resolve: func(err error) {
			result <- err
			close(result)
		},
	}

	err = r.enqueue(admissionContext, routeKey{kind: eventRoute, input: reflect.TypeFor[E]()}, job, try)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Runtime) enqueue(admissionContext context.Context, key routeKey, job queuedJob, try bool) error {
	configuredQueue, err := r.beginAdmission(key)
	if err != nil {
		return err
	}
	defer r.admissions.Done()

	if err := admissionContext.Err(); err != nil {
		configuredQueue.rejected.Add(1)
		return err
	}

	if try {
		select {
		case configuredQueue.jobs <- job:
			configuredQueue.accepted.Add(1)
			return nil
		default:
			configuredQueue.rejected.Add(1)
			return ErrQueueFull
		}
	}

	select {
	case configuredQueue.jobs <- job:
		configuredQueue.accepted.Add(1)
		return nil
	case <-admissionContext.Done():
		configuredQueue.rejected.Add(1)
		return admissionContext.Err()
	case <-r.closing:
		configuredQueue.rejected.Add(1)
		return ErrRuntimeClosed
	}
}

func (r *Runtime) beginAdmission(key routeKey) (*queue, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	queueName := DefaultQueueName
	if routedQueue, ok := r.routes[key]; ok {
		queueName = routedQueue
	}
	configuredQueue := r.queues[queueName]

	switch r.state {
	case StateRunning:
		r.admissions.Add(1)
		return configuredQueue, nil
	case StateNew:
		configuredQueue.rejected.Add(1)
		return nil, ErrRuntimeNotStarted
	case StateClosing, StateClosed:
		configuredQueue.rejected.Add(1)
		return nil, ErrRuntimeClosed
	default:
		configuredQueue.rejected.Add(1)
		return nil, ErrRuntimeClosed
	}
}
