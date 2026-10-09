package workers

import (
	"syscall"

	"github.com/turbokast/mythhelm/internal/contain"
)

// containAttr is nativeAttr plus the user and mount namespaces __contain
// builds the boundary in; the process keeps its own group, so the stop ladder
// is unchanged (I06).
func containAttr() (*syscall.SysProcAttr, error) {
	attr, ns := nativeAttr(), contain.NamespaceAttr()
	attr.Cloneflags, attr.UidMappings, attr.GidMappings = ns.Cloneflags, ns.UidMappings, ns.GidMappings
	return attr, nil
}
