package main

import "errors"

func live() error { return errors.New("camera Linux only; use --replay offline") }
