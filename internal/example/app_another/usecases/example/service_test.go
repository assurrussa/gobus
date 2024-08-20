package example_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/assurrussa/gobus"
	examplein "github.com/assurrussa/gobus/internal/example/app/application/port/in"
	"github.com/assurrussa/gobus/internal/example/app_another/commands/liveasync"
	"github.com/assurrussa/gobus/internal/example/app_another/commands/lucky"
	"github.com/assurrussa/gobus/internal/example/app_another/usecases/example"
	mocksgobus "github.com/assurrussa/gobus/internal/mocks"
)

func TestService_Handle(t *testing.T) {
	// Arrange.
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockHandlerLive := mocksgobus.NewMockResultCommandExecutor[examplein.LiveIn, examplein.LiveOut](ctrl)
	mockHandlerLucky := mocksgobus.NewMockResultCommandExecutor[lucky.In, lucky.Out](ctrl)
	mockHandlerLiveAsync := mocksgobus.NewMockResultCommandExecutor[liveasync.In, liveasync.Out](ctrl)
	mockHandlerLiveAsync2 := mocksgobus.NewMockCommandExecutor[liveasync.InAsync](ctrl)
	gobus.RegisterResult[examplein.LiveIn, examplein.LiveOut](mockHandlerLive)
	gobus.RegisterResult[lucky.In, lucky.Out](mockHandlerLucky)
	gobus.RegisterResult[liveasync.In, liveasync.Out](mockHandlerLiveAsync)
	gobus.Register[liveasync.InAsync](mockHandlerLiveAsync2)
	s := example.Service{}

	mockHandlerLive.EXPECT().Execute(ctx, examplein.LiveIn{Val: 1234}).Return(examplein.LiveOut{Val: 1235}, nil).Times(1)
	mockHandlerLucky.EXPECT().Execute(ctx, lucky.In{Val: "1235"}).Return(lucky.Out{Val: "1236"}, nil).Times(1)
	mockHandlerLiveAsync.EXPECT().Execute(ctx, liveasync.In{Val: 1235}).Return(liveasync.Out{Val: 1237}, nil).Times(1)
	checkResult := atomic.Bool{}
	mockHandlerLiveAsync2.EXPECT().
		Execute(ctx, liveasync.InAsync{Val: 1235}).
		DoAndReturn(func(context.Context, liveasync.InAsync) error {
			checkResult.Store(true)
			return nil
		}).Times(1)

	// Action.
	out, err := s.Handle(ctx, example.RequestIn{Value: "1234"})

	// Assert.
	checkNoError(t, err)
	checkEqual(t, "1235_test_1236_1237", out.Value)
	checkEventually(t, func() bool {
		res := checkResult.Load()
		return res
	}, 5*time.Second, 10*time.Millisecond)
}

func checkNoError(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("check error: %v", err)
	}
}

func checkEqual(t *testing.T, value, expected any) {
	t.Helper()

	if expected != value {
		t.Fatalf("value not equal to expected value: %v != %v", value, expected)
	}
}

func checkEventually(t *testing.T, condition func() bool, waitFor time.Duration, tick time.Duration) bool {
	t.Helper()

	ch := make(chan bool, 1)

	timer := time.NewTimer(waitFor)
	defer timer.Stop()

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for tickCh := ticker.C; ; {
		select {
		case <-timer.C:
			t.Fatalf("condition never satisfied")
		case <-tickCh:
			tickCh = nil
			go func() {
				ch <- condition()
			}()
		case v := <-ch:
			if v {
				return true
			}
			tickCh = ticker.C
		}
	}
}
