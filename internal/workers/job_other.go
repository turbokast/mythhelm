//go:build !windows

package workers

import "github.com/turbokast/mythhelm/internal/adapter"

// Windows-only ownership has no effect on Unix launch or persistence.
type jobObject struct{}
type jobOps struct{}

func (*jobObject) close() error                         { return nil }
func (*launcher) prepareJob() error                     { return nil }
func (*launcher) failedLaunch() adapter.InterruptReport { return adapter.InterruptReport{} }
func launchError(error) string                          { return "" }
func (l *launcher) finishJob(p *ownedProc) error {
	pid := p.cmd.Process.Pid
	l.w.id.NativePID, l.w.id.NativePGID = &pid, p.pgid
	l.identityErr = l.w.writeIdentity()
	return nil
}
