package liveasync

import (
	"context"
)

type In struct {
	Val int
}

type Out struct {
	Val int
}

type Handler struct {
	val int
}

func NewHandler(val int) *Handler {
	return &Handler{val: val}
}

func (h *Handler) Execute(_ context.Context, dto In) (Out, error) {
	return Out{Val: h.val + dto.Val}, nil
}
