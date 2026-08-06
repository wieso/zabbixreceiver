package zabbixreceiver

import (
	"context"
	"math"
	"math/rand/v2"
	"time"
)

type timerFunc func(context.Context, time.Duration) error
type jitterFunc func(time.Duration) time.Duration

func runJob(
	ctx context.Context,
	config JobConfig,
	jitterLimit time.Duration,
	timer timerFunc,
	jitter jitterFunc,
	run func(context.Context) error,
	report func(error),
) {
	if !config.RunOnStart && waitFor(ctx, config.Interval, timer) != nil {
		return
	}

	for {
		if ctx.Err() != nil {
			return
		}
		if waitFor(ctx, jitter(jitterLimit), timer) != nil {
			return
		}

		runCtx, cancel := context.WithTimeout(ctx, config.Timeout)
		err := run(runCtx)
		cancel()
		if err != nil {
			report(err)
		}
		if ctx.Err() != nil || waitFor(ctx, config.Interval, timer) != nil {
			return
		}
	}
}

func waitFor(ctx context.Context, delay time.Duration, timer timerFunc) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := timer(ctx, delay); err != nil {
		return err
	}
	return ctx.Err()
}

func waitTimer(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func randomJitter(limit time.Duration) time.Duration {
	if limit <= 0 {
		return 0
	}
	if limit == time.Duration(math.MaxInt64) {
		return time.Duration(rand.Uint64() >> 1)
	}
	return time.Duration(rand.Int64N(int64(limit) + 1))
}
