package example

import (
	"context"
	"fmt"
	"strconv"

	"github.com/assurrussa/gobus"
	examplein "github.com/assurrussa/gobus/internal/example/app/application/port/in"
	"github.com/assurrussa/gobus/internal/example/app_another/commands/liveasync"
	"github.com/assurrussa/gobus/internal/example/app_another/commands/lucky"
)

type Service struct {
	bus *gobus.Bus
}

func NewService(bus *gobus.Bus) *Service {
	return &Service{bus: bus}
}

func (s *Service) Handle(ctx context.Context, dto RequestIn) (ResponseOut, error) {
	n, err := strconv.Atoi(dto.Value)
	if err != nil {
		return ResponseOut{}, fmt.Errorf("parse value: %w", err)
	}

	out, err := s.bus.DispatchResult[examplein.LiveOut](ctx, examplein.LiveIn{Val: n})
	if err != nil {
		return ResponseOut{}, err
	}

	outLucky, err := s.bus.DispatchResult[lucky.Out](ctx, lucky.In{Val: strconv.Itoa(out.Val)})
	if err != nil {
		return ResponseOut{}, err
	}

	ch := s.bus.DispatchResultAsync[liveasync.Out](ctx, liveasync.In{Val: out.Val})
	outLiveAsync := <-ch
	if outLiveAsync.Error != nil {
		return ResponseOut{}, outLiveAsync.Error
	}
	outLiveAsyncRes := strconv.Itoa(outLiveAsync.Result.Val)

	asyncCh := s.bus.DispatchAsync(ctx, liveasync.InAsync{Val: out.Val})
	if err := <-asyncCh; err != nil {
		return ResponseOut{}, err
	}

	return ResponseOut{
		Value: strconv.Itoa(out.Val) + "_test_" + outLucky.Val + "_" + outLiveAsyncRes,
	}, nil
}
