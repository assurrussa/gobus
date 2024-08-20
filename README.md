# GO CommandBus

A simple and easy to plug-in command bus for Go.

## Install

Use go get.
```sh
$ github.com/assurrussa/gobus
```

Then import the package into your own code:
```
import "github.com/assurrussa/gobus"
```

## Usage
```go
package main

import (
	"context"
	"log"

	"github.com/assurrussa/gobus"
)

type User struct {
	Name string
}

type GetUserHandler struct{}

func (h *GetUserHandler) Execute(_ context.Context, dto *User) (int, error) {
	log.Printf("user %s getting", dto.Name)

	return len(dto.Name), nil
}

type CreateUserHandler struct{}

func (h *CreateUserHandler) Execute(_ context.Context, dto *User) error {
	log.Printf("user %s created", dto.Name)

	return nil
}

func main() {
	ctx := context.Background()

	gobus.RegisterResult[*User, int](&GetUserHandler{})
	gobus.Register[*User](&CreateUserHandler{})

	// action sync.
	result, err := gobus.DispatchResult[*User, int](ctx, &User{"gobus test"})
	log.Printf("dispatch resulting command - result: %v, err: %v", result, err)

	err = gobus.Dispatch[*User](ctx, &User{"gobus test"})
	log.Printf("dispatch command - err: %v", err)

	// action async.
	out := <-gobus.DispatchResultAsync[*User, int](ctx, &User{"gobus test async"})
	log.Printf("dispatch resulting command async - result: %+v", out)

	err = <-gobus.DispatchAsync[*User](ctx, &User{"gobus test async"})
	log.Printf("dispatch command async - %v", err)
}

```

## License

This project is released under the MIT licence. See [LICENSE](https://github.com/assurrussa/gobus/blob/master/LICENSE) for more details.