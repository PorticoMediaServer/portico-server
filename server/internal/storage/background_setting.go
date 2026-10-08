package storage

import "errors"

// SetBackgroundTaskPriority applies the owner setting to future background
// children and admission. Active work is never interrupted when it changes.
func (s *Supervisor) SetBackgroundTaskPriority(value string) error {
	if s == nil {
		return errors.New("missing background supervisor")
	}
	switch value {
	case "lower":
		s.backgroundNormal.Store(false)
	case "normal":
		s.backgroundNormal.Store(true)
	default:
		return errors.New("invalid background task priority")
	}
	return nil
}

func (s *Supervisor) BackgroundTaskPriority() string {
	if s != nil && s.backgroundNormal.Load() {
		return "normal"
	}
	return "lower"
}
