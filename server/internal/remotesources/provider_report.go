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

// sourceOperationInUse is the live serve process's measured admitted-operation
// count for this exact source generation. One permit is one admitted provider
// operation on the process's shared source budget (a listing, stat or range),
// never a physical socket and never a distinct viewer. The snapshot is fenced
// against the same slot, generation and process before it is returned, so a
// replacement or retirement during the read reports not-ok rather than a stale
// count; no process for this generation means it has no admitted serve work.
func (s *Service) sourceOperationInUse(source, generation string) (int, bool) {
	if s == nil || source == "" || generation == "" {
		return 0, false
	}
	s.serveMu.Lock()
	slot := s.serveLeases[source]
	if slot == nil || slot.generation != generation || slot.CheckLocal() != nil {
		s.serveMu.Unlock()
		return 0, false
	}
	process := slot.process
	s.serveMu.Unlock()
	if process == nil {
		return 0, false
	}
	active := process.OperationCounts().Active
	s.serveMu.Lock()
	current := s.serveLeases[source]
	ok := current == slot && current.generation == generation && current.process == process && current.CheckLocal() == nil
	s.serveMu.Unlock()
	if !ok {
		return 0, false
	}
	return active, true
}
