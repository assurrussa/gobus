package in

import "context"

type LiveIn struct {
	Val int
}

type LiveOut struct {
	Val int
}

type LiveHandler interface {
	Handle(ctx context.Context, in LiveIn) (LiveOut, error)
}
