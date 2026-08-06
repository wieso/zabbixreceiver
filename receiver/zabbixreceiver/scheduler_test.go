package zabbixreceiver

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunJobRunOnStartDelaySequence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var events []string
	runs := 0
	runJob(ctx, JobConfig{RunOnStart: true, Interval: 10 * time.Second, Timeout: time.Minute}, 5*time.Second,
		func(_ context.Context, delay time.Duration) error {
			events = append(events, "wait:"+delay.String())
			return nil
		},
		func(limit time.Duration) time.Duration {
			require.Equal(t, 5*time.Second, limit)
			return 3 * time.Second
		},
		func(context.Context) error {
			events = append(events, "run")
			runs++
			if runs == 2 {
				cancel()
			}
			return nil
		},
		func(error) {},
	)

	assert.Equal(t, []string{"wait:3s", "run", "wait:10s", "wait:3s", "run"}, events)
}

func TestRunJobWithoutRunOnStartWaitsIntervalThenJitter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var events []string
	runJob(ctx, JobConfig{Interval: 10 * time.Second, Timeout: time.Minute}, 5*time.Second,
		func(_ context.Context, delay time.Duration) error {
			events = append(events, "wait:"+delay.String())
			return nil
		},
		func(time.Duration) time.Duration { return 3 * time.Second },
		func(context.Context) error {
			events = append(events, "run")
			cancel()
			return nil
		},
		func(error) {},
	)

	assert.Equal(t, []string{"wait:10s", "wait:3s", "run"}, events)
}

func TestRunJobUsesFreshTimeoutContextForEveryRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const timeout = time.Minute
	var runContexts []context.Context
	runJob(ctx, JobConfig{RunOnStart: true, Interval: time.Second, Timeout: timeout}, 0,
		func(context.Context, time.Duration) error { return nil },
		func(time.Duration) time.Duration { return 0 },
		func(runCtx context.Context) error {
			runContexts = append(runContexts, runCtx)
			deadline, ok := runCtx.Deadline()
			require.True(t, ok)
			remaining := time.Until(deadline)
			assert.Greater(t, remaining, timeout/2)
			assert.LessOrEqual(t, remaining, timeout)
			if len(runContexts) == 2 {
				cancel()
			}
			return nil
		},
		func(error) {},
	)

	require.Len(t, runContexts, 2)
	assert.NotEqual(t, runContexts[0], runContexts[1])
	for _, runCtx := range runContexts {
		select {
		case <-runCtx.Done():
		default:
			t.Error("run context was not canceled after the run returned")
		}
	}
}

func TestRunJobCancellationStopsBeforeRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jitterCalls := 0
	runs := 0
	runJob(ctx, JobConfig{Interval: time.Second, Timeout: time.Second}, time.Second,
		func(context.Context, time.Duration) error {
			cancel()
			return nil
		},
		func(time.Duration) time.Duration {
			jitterCalls++
			return 0
		},
		func(context.Context) error {
			runs++
			return nil
		},
		func(error) {},
	)

	assert.Zero(t, jitterCalls)
	assert.Zero(t, runs)
}

func TestRunJobDoesNotOverlapBlockedRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan struct{})
	var active atomic.Int32
	var maximum atomic.Int32
	var runs atomic.Int32

	go func() {
		defer close(done)
		runJob(ctx, JobConfig{RunOnStart: true, Interval: time.Second, Timeout: time.Minute}, 0,
			func(context.Context, time.Duration) error { return nil },
			func(time.Duration) time.Duration { return 0 },
			func(context.Context) error {
				current := active.Add(1)
				defer active.Add(-1)
				for {
					observed := maximum.Load()
					if current <= observed || maximum.CompareAndSwap(observed, current) {
						break
					}
				}
				started <- struct{}{}
				if runs.Add(1) == 1 {
					<-release
				} else {
					cancel()
				}
				return nil
			},
			func(error) {},
		)
	}()

	requireReceive(t, started, "first run did not start")
	select {
	case <-started:
		cancel()
		close(release)
		requireReceive(t, done, "scheduler did not stop after overlap")
		t.Fatal("a second run started while the first run was blocked")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	requireReceive(t, started, "second run did not start after release")
	requireReceive(t, done, "scheduler did not stop")
	assert.Equal(t, int32(1), maximum.Load())
}

func TestRunJobReportsErrorsAndContinues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wantErr := errors.New("temporary failure")
	var reports []error
	runs := 0
	runJob(ctx, JobConfig{RunOnStart: true, Interval: time.Second, Timeout: time.Second}, 0,
		func(context.Context, time.Duration) error { return nil },
		func(time.Duration) time.Duration { return 0 },
		func(context.Context) error {
			runs++
			if runs == 1 {
				return wantErr
			}
			cancel()
			return nil
		},
		func(err error) { reports = append(reports, err) },
	)

	assert.Equal(t, 2, runs)
	assert.Equal(t, []error{wantErr}, reports)
}

func requireReceive(t *testing.T, channel <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-channel:
	case <-time.After(time.Second):
		require.Fail(t, failure, fmt.Sprintf("timed out after %s", time.Second))
	}
}
