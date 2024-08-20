package live

import (
	"context"

	"github.com/assurrussa/gobus/internal/example/app/application/port/in"
)

type Handler struct {
	val int
}

func NewHandler(a int) *Handler {
	return &Handler{val: a}
}

func (h *Handler) Handle(_ context.Context, dto in.LiveIn) (in.LiveOut, error) {
	return in.LiveOut{Val: h.val + dto.Val}, nil
}
