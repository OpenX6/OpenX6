package main

import (
	"errors"
	"strings"
	"testing"
)

func TestMaintenanceBoundToStoredIdentity(t *testing.T) {
	h := strings.Repeat("a", 64)
	if !maintenanceReady(h, h, nil, true) {
		t.Fatal("valid enrollment refused")
	}
	if maintenanceReady(h, h, nil, false) || maintenanceReady("", "", nil, true) || maintenanceReady(h, strings.Repeat("b", 64), nil, true) || maintenanceReady(h, h, errors.New("read"), true) {
		t.Fatal("invalid enrollment accepted")
	}
}
