package liveasync

import (
	"context"
	"log/slog"
)

type InAsync struct {
	Val int
}

type HandlerAsync struct {
	logger *slog.Logger
	val    int
}

func NewHandlerAsync(val int, logger *slog.Logger) *HandlerAsync {
	return &HandlerAsync{val: val, logger: logger}
}

func (h *HandlerAsync) Execute(_ context.Context, dto InAsync) error {
	h.logger.Warn("async HandlerAsync dto", slog.Int("val", dto.Val))

	return nil
}
