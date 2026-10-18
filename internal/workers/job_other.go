//go:build !windows

package workers

// Windows-only ownership has no effect on Unix launch or persistence.
type jobObject struct{}
type jobOps struct{} //nolint:unused // Shared worker field supplies the Windows-only launch seam.

func (*jobObject) close() error     { return nil }
func (*launcher) prepareJob() error { return nil }
func (*launcher) failedLaunch()     {}
func launchError(error) string      { return "" }
func (l *launcher) finishJob(p *ownedProc) error {
	pid := p.cmd.Process.Pid
	l.w.id.NativePID, l.w.id.NativePGID = &pid, p.pgid
	l.identityErr = l.w.writeIdentity()
	return nil
}
