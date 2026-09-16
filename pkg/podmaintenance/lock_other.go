//go:build !linux && !darwin

package podmaintenance

import "errors"

func lock(string) (func(), error) {
	return nil, errors.New("pod maintenance is unsupported on this OS")
}
