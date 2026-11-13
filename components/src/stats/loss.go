package main

import "strings"

// Counters are cumulative for the process; compare against the last idle/enable snapshot.
func lossSince(base, now map[string]uint64) (output, input bool, reason string) {
	if len(base) == 0 || len(base) != len(now) {
		return true, true, "counter_set_changed"
	}
	for k, n := range now {
		b, ok := base[k]
		if !ok || n < b {
			return true, true, "counter_reset"
		}
		if n == b {
			continue
		}
		if strings.HasPrefix(k, "input:") {
			input = true
		} else {
			output = true
			reason = k
		}
	}
	return
}
