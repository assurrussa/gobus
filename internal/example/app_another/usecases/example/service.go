package example

import (
	"context"
	"strconv"

	"github.com/assurrussa/gobus"
	examplein "github.com/assurrussa/gobus/internal/example/app/application/port/in"
	"github.com/assurrussa/gobus/internal/example/app_another/commands/liveasync"
	"github.com/assurrussa/gobus/internal/example/app_another/commands/lucky"
)

type Service struct{}

func (s *Service) Handle(ctx context.Context, dto RequestIn) (ResponseOut, error) {
	n, _ := strconv.Atoi(dto.Value)

	out, err := gobus.DispatchResult[examplein.LiveIn, examplein.LiveOut](ctx, examplein.LiveIn{Val: n})
	if err != nil {
		return ResponseOut{}, err
	}

	outLucky, err := gobus.DispatchResult[lucky.In, lucky.Out](ctx, lucky.In{Val: strconv.Itoa(out.Val)})
	if err != nil {
		return ResponseOut{}, err
	}

	ch := gobus.DispatchResultAsync[liveasync.In, liveasync.Out](ctx, liveasync.In{Val: out.Val})
	outLiveAsync := <-ch
	if outLiveAsync.Error != nil {
		return ResponseOut{}, outLiveAsync.Error
	}
	outLiveAsyncRes := strconv.Itoa(outLiveAsync.Result.Val)

	gobus.DispatchAsync[liveasync.InAsync](ctx, liveasync.InAsync{Val: out.Val})

	return ResponseOut{
		Value: strconv.Itoa(out.Val) + "_test_" + outLucky.Val + "_" + outLiveAsyncRes,
	}, nil
}
