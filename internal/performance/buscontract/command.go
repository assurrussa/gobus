package bus

import (
	"context"
)

//go:generate mockgen -source=command.go -destination=./command_mock.go -package=bus

type Command[Q objectIn, T objectOut] interface {
	Execute(ctx context.Context, dto Q) (T, error)
}
