package runner

import "context"

type Process interface {
	Start(context.Context) error
	Stop(context.Context) error
}

type ProcessHook interface {
	Process

	RegisterRunnerHook(Hook)
}

type Hook interface {
	Shutdown(error)
}
