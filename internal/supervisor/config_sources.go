package supervisor

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
)

// exactBoundConfigSources projects the pinned adapter's available
// source→path inventory onto the admitted manifest: the result holds exactly
// the admitted sources, each with its inventoried path. The inventory may
// list candidates the manifest never admitted; they are dropped, never
// forwarded. An admitted source the inventory lacks, or whose path is empty
// or not absolute, refuses the launch (I20). An empty manifest maps nothing.
func exactBoundConfigSources(available, admitted map[string]string) (map[string]string, error) {
	if len(admitted) == 0 {
		return nil, nil
	}
	bound := make(map[string]string, len(admitted))
	for _, source := range slices.Sorted(maps.Keys(admitted)) {
		path, ok := available[source]
		if !ok {
			return nil, fmt.Errorf("admitted native config %q is not in the adapter inventory", source)
		}
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("admitted native config %q has no absolute inventoried path", source)
		}
		bound[source] = path
	}
	return bound, nil
}
