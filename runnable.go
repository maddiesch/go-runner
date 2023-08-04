package runner

import "context"

type Runnable interface {
	Start(context.Context) error
	Stop(context.Context) error
}

type RunnableHook interface {
	Runnable

	RegisterRunnerHook(Hook)
}

type Hook interface {
	Shutdown(error)
}
