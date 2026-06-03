package fault

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/middleware"
)

// WithFaultInjection returns a Middleware that injects faults according to cfg.
func WithFaultInjection(cfg Config) middleware.Middleware {
	if len(cfg.ErrorTypes) == 0 {
		cfg.ErrorTypes = []ErrorType{Unavailable}
	}
	var seed int64
	if cfg.Seed != 0 {
		seed = cfg.Seed
	} else {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))
	var mu sync.Mutex

	return func(next core.Runnable) core.Runnable {
		return core.NewRunnableFunc(next.Name(), func(ctx context.Context, input any) (any, error) {
			if cfg.LatencyP99 > 0 {
				mu.Lock()
				jitter := time.Duration(rng.Int63n(int64(cfg.LatencyP99)))
				mu.Unlock()
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(jitter):
				}
			}

			mu.Lock()
			shouldFail := cfg.ErrorRate > 0 && rng.Float64() < cfg.ErrorRate
			var errType ErrorType
			if shouldFail {
				errType = cfg.ErrorTypes[rng.Intn(len(cfg.ErrorTypes))]
			}
			mu.Unlock()

			if shouldFail {
				if errType == Timeout {
					return nil, context.DeadlineExceeded
				}
				return nil, errorMessages[errType]
			}

			return next.Invoke(ctx, input)
		})
	}
}
