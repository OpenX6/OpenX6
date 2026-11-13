//go:build !linux

package main

import "fmt"

func stopInsUSB() error { return fmt.Errorf("receiver lifecycle requires Linux") }
