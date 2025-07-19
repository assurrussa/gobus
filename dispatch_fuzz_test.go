package gobus_test

import (
	"context"
	"testing"

	"github.com/assurrussa/gobus"
)

func FuzzBusTypeSnapshots(f *testing.F) {
	f.Add(int64(0), "")
	f.Add(int64(42), "gobus")
	f.Add(int64(-1), "type snapshot")

	f.Fuzz(func(t *testing.T, number int64, text string) {
		ctx := context.Background()
		bus := gobus.New()
		bus.Register(fuzzInputCommandHandler{})
		bus.Register(fuzzPointerCommandHandler{})
		bus.RegisterResult(fuzzNumberResultHandler{})
		bus.RegisterResult(fuzzTextResultHandler{})
		bus.RegisterResult(fuzzPointerResultHandler{})

		in := fuzzInput{number: number, text: text}
		if err := bus.Dispatch(ctx, in); err != nil {
			t.Fatalf("dispatch value command: %v", err)
		}
		if err := bus.Dispatch(ctx, &in); err != nil {
			t.Fatalf("dispatch pointer command: %v", err)
		}

		numberOut, err := bus.DispatchResult[int64](ctx, in)
		if err != nil {
			t.Fatalf("dispatch number result: %v", err)
		}
		if numberOut != number {
			t.Fatalf("number result = %d, want %d", numberOut, number)
		}

		textOut, err := bus.DispatchResult[string](ctx, in)
		if err != nil {
			t.Fatalf("dispatch text result: %v", err)
		}
		if textOut != text {
			t.Fatalf("text result = %q, want %q", textOut, text)
		}

		pointerOut, err := bus.DispatchResult[fuzzOutput](ctx, &in)
		if err != nil {
			t.Fatalf("dispatch pointer result: %v", err)
		}
		if pointerOut != (fuzzOutput{number: number, text: text}) {
			t.Fatalf("pointer result = %+v, want input values", pointerOut)
		}
	})
}

type fuzzInput struct {
	number int64
	text   string
}

type fuzzOutput struct {
	number int64
	text   string
}

type fuzzInputCommandHandler struct{}

func (fuzzInputCommandHandler) Execute(_ context.Context, _ fuzzInput) error {
	return nil
}

type fuzzPointerCommandHandler struct{}

func (fuzzPointerCommandHandler) Execute(_ context.Context, _ *fuzzInput) error {
	return nil
}

type fuzzNumberResultHandler struct{}

func (fuzzNumberResultHandler) Execute(_ context.Context, dto fuzzInput) (int64, error) {
	return dto.number, nil
}

type fuzzTextResultHandler struct{}

func (fuzzTextResultHandler) Execute(_ context.Context, dto fuzzInput) (string, error) {
	return dto.text, nil
}

type fuzzPointerResultHandler struct{}

func (fuzzPointerResultHandler) Execute(_ context.Context, dto *fuzzInput) (fuzzOutput, error) {
	return fuzzOutput{number: dto.number, text: dto.text}, nil
}
