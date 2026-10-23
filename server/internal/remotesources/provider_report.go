package remotesources

// sourcePacerBackoff projects only the actual current serve process's report.
// It does not infer provider state from capacity contention or a loopback HTTP
// response, persist private log text, create a deadline or authorize any work.
func (s *Service) sourcePacerBackoff(source, generation string) bool {
	s.serveMu.Lock()
	slot := s.serveLeases[source]
	if slot == nil || slot.generation != generation || slot.CheckLocal() != nil {
		s.serveMu.Unlock()
		return false
	}
	process := slot.process
	s.serveMu.Unlock()
	return process != nil && process.PacerObservation().Backoff
}
