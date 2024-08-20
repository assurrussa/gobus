package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/assurrussa/gobus"
	"github.com/assurrussa/gobus/internal/example/api"
	exampleusecaseliveadapter "github.com/assurrussa/gobus/internal/example/app/adapter/usecases/live"
	examplein "github.com/assurrussa/gobus/internal/example/app/application/port/in"
	exampleusecaselive "github.com/assurrussa/gobus/internal/example/app/application/usecases/live"
	"github.com/assurrussa/gobus/internal/example/app_another/commands/liveasync"
	"github.com/assurrussa/gobus/internal/example/app_another/commands/lucky"
	"github.com/assurrussa/gobus/internal/example/app_another/usecases/example"
)

func main() {
	logger := slog.Default()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	useCaseLive := exampleusecaselive.NewHandler(3)
	adapterUseCaseLive := exampleusecaseliveadapter.NewAdapter(useCaseLive)
	gobus.RegisterResult[examplein.LiveIn, examplein.LiveOut](adapterUseCaseLive)
	gobus.RegisterResult[lucky.In, lucky.Out](lucky.NewHandler("test"))
	gobus.RegisterResult[liveasync.In, liveasync.Out](liveasync.NewHandler(1234))
	gobus.Register[liveasync.InAsync](liveasync.NewHandlerAsync(234, logger))

	rw := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	api.NewExampleController(&example.Service{}, logger).ExampleHandler()(rw, req)

	response := rw.Result()
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(response.Body)
	logger.Info("app response", slog.Any("body", body))

	go func() {
		time.Sleep(time.Second)
		cancel()
	}()

	<-ctx.Done()
	logger.Info("app finished!")
}
