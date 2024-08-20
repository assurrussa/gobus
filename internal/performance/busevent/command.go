package busevent

import (
	"context"
)

//go:generate mockgen -source=command.go -destination=./command_mock.go -package=busevent

type Command interface {
	Execute(ctx context.Context, dto any) (any, error)
}
