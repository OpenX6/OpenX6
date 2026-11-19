//go:build !darwin

package update

func matchingReadyBundle(target, running string) (bool, error) { return false, nil }
