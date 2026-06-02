package fault

import (
	"context"
	"math/rand"
	"time"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/middleware"
)

// WithFaultInjection returns a Middleware that injects faults according to cfg.
func WithFaultInjection(cfg Config) middleware.Middleware {
	if len(cfg.ErrorTypes) == 0 {
		cfg.ErrorTypes = []ErrorType{Unavailable}
	}
	var rng *rand.Rand
	if cfg.Seed != 0 {
		rng = rand.New(rand.NewSource(cfg.Seed))
	} else {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	return func(next core.Runnable) core.Runnable {
		return core.NewRunnableFunc(next.Name(), func(ctx context.Context, input any) (any, error) {
			if cfg.LatencyP99 > 0 {
				jitter := time.Duration(rng.Int63n(int64(cfg.LatencyP99)))
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(jitter):
				}
			}

			if cfg.ErrorRate > 0 && rng.Float64() < cfg.ErrorRate {
				errType := cfg.ErrorTypes[rng.Intn(len(cfg.ErrorTypes))]
				if errType == Timeout {
					return nil, context.DeadlineExceeded
				}
				return nil, errorMessages[errType]
			}

			return next.Invoke(ctx, input)
		})
	}
}
