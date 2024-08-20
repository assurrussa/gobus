package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/assurrussa/gobus/internal/example/app_another/usecases/example"
)

type useCaseExample interface {
	Handle(ctx context.Context, dto example.RequestIn) (example.ResponseOut, error)
}

type ExampleController struct {
	logger         *slog.Logger
	useCaseExample useCaseExample
}

func NewExampleController(useCaseExample useCaseExample, logger *slog.Logger) *ExampleController {
	return &ExampleController{useCaseExample: useCaseExample, logger: logger}
}

func (c *ExampleController) ExampleHandler() func(http.ResponseWriter, *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		out, err := c.useCaseExample.Handle(ctx, example.RequestIn{
			Value: "123",
		})
		if err != nil {
			c.logger.Error("could not handle example", slog.Any("err", err))
			res.WriteHeader(http.StatusInternalServerError)
			_, _ = res.Write([]byte("Oops, something went wrong!"))
			return
		}

		res.WriteHeader(http.StatusOK)
		_, _ = res.Write([]byte(out.Value))
	}
}
