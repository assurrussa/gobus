package async

import (
	"context"
	"fmt"
)

type submitOptionKind uint8

const executionContextOption submitOptionKind = 1

// SubmitOption configures admission-independent behavior for a submitted job.
// The zero value is invalid.
type SubmitOption struct {
	kind             submitOptionKind
	executionContext context.Context //nolint:containedctx // The option transfers this context into an accepted job.
}

// WithExecutionContext stores ctx with an accepted job and uses it for handler
// execution instead of the admission context. Its deadline starts when ctx is
// created, not when a worker starts the job. When repeated, the last option wins.
func WithExecutionContext(ctx context.Context) SubmitOption {
	return SubmitOption{kind: executionContextOption, executionContext: ctx}
}

func executionContextFor(admissionContext context.Context, options []SubmitOption) (context.Context, error) {
	executionContext := admissionContext
	for index, option := range options {
		switch option.kind {
		case executionContextOption:
			executionContext = option.executionContext
		default:
			return nil, fmt.Errorf("%w: option %d", ErrInvalidSubmitOption, index)
		}
	}
	if executionContext == nil {
		return nil, fmt.Errorf("%w: execution context is nil", ErrInvalidSubmitOption)
	}
	return executionContext, nil
}
