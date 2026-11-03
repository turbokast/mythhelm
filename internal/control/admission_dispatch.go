package control

// RegisterPreparedAdmission atomically binds exactly the two staged admission
// methods. Their handlers own bounded volatile replay and approved durable
// acknowledgements; generic Execute must never retain an unapproved challenge.
func (s *Server) RegisterPreparedAdmission(prepare Handler, confirm Handler) error {
	if s == nil || prepare == nil || confirm == nil {
		return newError(CodeInvalidContract, "two admission handlers are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.prepared || s.methods["run.prepare"] != nil || s.methods["run.confirm"] != nil {
		return newError(CodeRevisionConflict, "admission handlers already registered")
	}
	if s.methods == nil {
		s.methods = map[string]Handler{}
	}
	s.methods["run.prepare"], s.methods["run.confirm"] = prepare, confirm
	s.prepared = true
	return nil
}

// PreparedAdmissionDigest uses the existing exact intent digest algorithm.
// It identifies an operation's arguments and never supplies authority.
func PreparedAdmissionDigest(intent Intent) (string, error) {
	if err := intent.Validate(); err != nil {
		return "", newError(CodeInvalidContract, "invalid admission intent")
	}
	if intent.Method != "run.prepare" && intent.Method != "run.confirm" {
		return "", newError(CodeCapabilityUnsupported, "method has no prepared-admission digest")
	}
	return intentDigest(intent), nil
}
