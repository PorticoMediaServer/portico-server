package subtitles

import "context"

// Window inputs retain the existing observed source and private endpoint
// confinement. Ordinary HLS owns the plan, command graph, output and admission.
type WindowInput interface {
	Run(context.Context, string, func(string) ([]string, error)) error
	Close() error
}
type WindowInputs interface {
	OpenPlaybackWindow(context.Context, string) (WindowInput, error)
}
