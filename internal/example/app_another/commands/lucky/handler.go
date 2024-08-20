package lucky

import "context"

type In struct {
	Val string
}

type Out struct {
	Val string
}

type Handler struct {
	val string
}

func NewHandler(a string) *Handler {
	return &Handler{val: a}
}

func (h *Handler) Execute(_ context.Context, dto In) (Out, error) {
	return Out{Val: h.val + dto.Val}, nil
}
