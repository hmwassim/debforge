package ports

import "context"

type Locker interface {
	Acquire(ctx context.Context, name string) (release func(), err error)
}
