package workers

// OrphanPIDs reports Linux processes with the attempt's exact environment
// marker. It never signals them; a launch-window crash is still unresolved
// even when no readable matching process is found.
func OrphanPIDs(attemptID string) ([]int, error) { return markedPIDs(AttemptMarker(attemptID)) }
