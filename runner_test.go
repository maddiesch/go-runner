package runner_test

import (
	"context"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/maddiesch/go-runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testRunnable struct {
	started uint32
	stopped uint32
}

func (t *testRunnable) Start(context.Context) error {
	atomic.AddUint32(&t.started, 1)
	return nil
}

func (t *testRunnable) Stop(context.Context) error {
	atomic.AddUint32(&t.stopped, 1)
	return nil
}

type testHookRunnable struct {
	*testRunnable

	shutdown func(error)
}

func (t *testHookRunnable) RegisterRunnerHook(h runner.Hook) {
	t.shutdown = h.Shutdown
}

func TestRunner(t *testing.T) {
	t.Run("Start & Stop", func(t *testing.T) {
		tr := &testRunnable{}

		run := runner.New(tr)

		err := run.Start(context.Background())
		require.NoError(t, err)

		err = run.Stop(context.Background())
		require.NoError(t, err)

		assert.Equal(t, uint32(1), tr.started)
		assert.Equal(t, uint32(1), tr.stopped)
	})

	t.Run("Signal Handler", func(t *testing.T) {
		tr := &testRunnable{}

		run := runner.New(tr)
		run.SetSignal(syscall.SIGUSR1)

		err := run.Start(context.Background())
		require.NoError(t, err)

		go func() {
			p, err := os.FindProcess(syscall.Getpid())
			require.NoError(t, err)

			p.Signal(syscall.SIGUSR1)
		}()

		select {
		case <-run.Wait():
		case <-time.After(100 * time.Millisecond):
			assert.Fail(t, "wait timeout")
		}

		err = run.Stop(context.Background())
		require.NoError(t, err)

		assert.Equal(t, uint32(1), tr.started)
		assert.Equal(t, uint32(1), tr.stopped)
	})

	t.Run("Hook Shutdown", func(t *testing.T) {
		tr := &testHookRunnable{&testRunnable{}, nil}

		run := runner.New(tr)

		err := run.Start(context.Background())
		require.NoError(t, err)

		go tr.shutdown(nil)

		select {
		case <-run.Wait():
		case <-time.After(100 * time.Millisecond):
			assert.Fail(t, "wait timeout")
		}

		err = run.Stop(context.Background())
		require.NoError(t, err)

		assert.Equal(t, uint32(1), tr.started)
		assert.Equal(t, uint32(1), tr.stopped)
	})
}
