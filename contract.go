package gobus

import (
	"context"
)

//go:generate mockgen -source=contract.go -destination=./internal/mocks/contract_mock.go -package=mocksgobus

type ResultCommandExecutor[Q ObjectIn, T ObjectOut] interface {
	Execute(ctx context.Context, dto Q) (T, error)
}

type CommandExecutor[Q ObjectIn] interface {
	Execute(ctx context.Context, dto Q) error
}
