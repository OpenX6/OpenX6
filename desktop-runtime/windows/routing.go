package main

import (
	"errors"
	"fmt"
	"strings"
)

// Preserve registered workers with compatible toolchains across desktop upgrades.
const originalBase = "e59263c6a0f673fa68196825fd5ad0685cc37a11731dca6b0cc4d66d82bfb0b1"
const factory827Base = "d8c99be83e1edcd47cd636e1356bf29313f079c35b5a40c9760e89ff302e399e"
const releaseBase = "24fe8d0d14c3ef01904969a4efab41b539c435f68ebfbd31ff6c6c74444911e9"
const distro = "OpenX6-Builder-v1"

func supportsBase(installed, packaged string) bool {
	if packaged == "" || installed == "" {
		return false
	}
	switch packaged {
	case releaseBase:
		return installed == releaseBase || installed == factory827Base || installed == originalBase
	case factory827Base:
		return installed == factory827Base || installed == originalBase
	default:
		return installed == packaged
	}
}

func prepare(registered bool, packaged string, version func() (string, error), install func() error) (string, error) {
	mode := "existing"
	if !registered {
		if err := install(); err != nil {
			return "", err
		}
		mode = "imported"
	}
	v, err := version()
	if err != nil {
		return "", fmt.Errorf("cannot read the registered OpenX6 worker; keep it and its Holder: %w", err)
	}
	if !supportsBase(strings.TrimSpace(v), packaged) {
		return "", errors.New("unsupported OpenX6 WSL base; keep the distribution and its Holder")
	}
	return mode, nil
}
