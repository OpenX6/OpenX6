//go:build !linux

package main

func watchPresets(string) (<-chan struct{}, func()) { return nil, func() {} }
