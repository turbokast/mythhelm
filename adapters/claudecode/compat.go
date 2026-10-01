package claudecode

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrCapability is a version/platform refusal, mapped to CLI exit 7.
var ErrCapability = errors.New("native capability unavailable")

func compatibility(version string, allowUntested bool) (string, error) {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("%w: native_version_unrecognised", ErrCapability)
	}
	numbers := make([]int, 3)
	for i, s := range parts {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return "", fmt.Errorf("%w: native_version_unrecognised", ErrCapability)
		}
		numbers[i] = n
	}
	if numbers[0] == 2 && numbers[1] == 1 && numbers[2] >= 284 {
		if version == "2.1.284" {
			return "fixture-tested on 2.1.284", nil
		}
		return "fixture-tested on 2.1.284; running " + version + " untested", nil
	}
	if allowUntested {
		return "experimental: fixture-tested on 2.1.284; running " + version + " untested", nil
	}
	return "", fmt.Errorf("%w: native_version_untested (%s): use --allow-untested-native-version only after reviewing compatibility", ErrCapability, version)
}
