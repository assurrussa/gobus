package live

import (
	"context"

	"github.com/assurrussa/gobus/internal/example/app/application/port/in"
)

type Adapter struct {
	handler in.LiveHandler
}

func NewAdapter(handler in.LiveHandler) *Adapter {
	return &Adapter{handler: handler}
}

func (h *Adapter) Execute(ctx context.Context, dto in.LiveIn) (in.LiveOut, error) {
	return h.handler.Handle(ctx, dto)
}
