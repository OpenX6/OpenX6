//go:build !linux

package main

import "os"

// Host directory-model tests do not implement or claim target SELinux behavior.
func preserveSELinux(old, new *os.File) error { return nil }
