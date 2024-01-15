package runner

import (
	"context"
	"os"
	"os/signal"
	"runtime"
	"sync"

	"github.com/maddiesch/go-bus"
)

type Runner struct {
	process Process
	stopped context.CancelFunc
	started bool
	signal  []os.Signal
	wait    []chan Status
	mu      sync.Mutex
	workers sync.WaitGroup
	hooks   *eventBusHook
}

// Create a new "Process" runner instance.
func New(p Process) *Runner {
	return &Runner{
		process: p,
		signal:  []os.Signal{os.Interrupt},
		hooks: &eventBusHook{
			Bus: bus.New[*busEvent](),
		},
	}
}

func (r *Runner) finishWaiter(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, ch := range r.wait {
		ch <- &waitStatus{err}
		close(ch)
	}
	r.wait = nil
}

func (r *Runner) SetSignal(sigs ...os.Signal) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		panic("runner already started")
	}

	r.signal = sigs
}

func (r *Runner) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.started = true

	rCtx, cancel := context.WithCancel(context.Background())
	r.stopped = cancel

	r.workers.Add(2)

	var wg sync.WaitGroup
	wg.Add(2)

	go r.startInteruptHandler(rCtx, wg.Done)
	go r.startEventBustHandler(rCtx, wg.Done)

	wg.Wait()

	if hooker, ok := r.process.(ProcessHook); ok {
		hooker.RegisterRunnerHook(r.hooks)
	}

	return r.process.Start(rCtx)
}

func (r *Runner) Stop(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stopped()
	r.stopped = nil

	r.workers.Wait()

	return r.process.Stop(ctx)
}

func (r *Runner) Wait() <-chan Status {
	ch := make(chan Status, 1)

	r.mu.Lock()
	r.wait = append(r.wait, ch)
	r.mu.Unlock()

	return ch
}

func (r *Runner) startInteruptHandler(ctx context.Context, started func()) {
	go started()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, r.signal...)

	defer r.workers.Done()
	defer func() {
		signal.Stop(signalChan)
		close(signalChan)
	}()

	select {
	case <-ctx.Done():
		runtime.Goexit()
	case s := <-signalChan:
		go r.signalHandlerEventReceived(s)
	}
}

func (r *Runner) startEventBustHandler(ctx context.Context, started func()) {
	go started()

	defer r.workers.Done()

	listener, cancel := r.hooks.Sink()
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			runtime.Goexit()
		case event := <-listener:
			r.handleBusEvent(event)
		}
	}
}

func (r *Runner) handleBusEvent(event *busEvent) {
	switch event.name {
	case busEventNameShutdown:
		if err, ok := event.payload.(error); ok {
			r.finishWaiter(err)
		} else {
			r.finishWaiter(nil)
		}
	}
}

func (r *Runner) signalHandlerEventReceived(sig os.Signal) {
	r.finishWaiter(nil)
}

type Status interface {
	Err() error
	ExitCode() int
}

type busEvent struct {
	name    string
	payload any
}

const (
	busEventNameShutdown = "runner.busEvent.shutdown"
)

type eventBusHook struct {
	*bus.Bus[*busEvent]
}

func (h *eventBusHook) Shutdown(err error) {
	h.Publish(&busEvent{name: busEventNameShutdown, payload: err})
}

var _ Hook = (*eventBusHook)(nil)

type waitStatus struct {
	err error
}

func (w *waitStatus) Err() error {
	return w.err
}

func (w *waitStatus) ExitCode() int {
	if w.err != nil {
		return 1
	}
	return 0
}

var _ Status = (*waitStatus)(nil)
