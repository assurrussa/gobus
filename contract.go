package gobus

import (
	"context"
)

//go:generate mockgen -source=contract.go -destination=./internal/mocks/contract_mock.go -package=mocksgobus

// ResultCommandExecutor handles a query of type Q and returns a result of type T.
type ResultCommandExecutor[Q ObjectIn, T ObjectOut] interface {
	Execute(ctx context.Context, dto Q) (T, error)
}

// CommandExecutor handles a command of type Q.
type CommandExecutor[Q ObjectIn] interface {
	Execute(ctx context.Context, dto Q) error
}
