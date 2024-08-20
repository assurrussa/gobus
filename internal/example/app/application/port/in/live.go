package in

import "context"

type LiveIn struct {
	Val int
}

type LiveOut struct {
	Val int
}

type LiveHandler interface {
	Handle(context.Context, LiveIn) (LiveOut, error)
}
