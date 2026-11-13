package main

import (
	"os"
	"path/filepath"
	"time"
)

func startFraming(root, settings string) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	events, cancel := watchPresets(settings)
	go func() {
		defer close(done)
		var model guiReader
		defer model.close()
		sample := time.NewTicker(40 * time.Millisecond)
		defer sample.Stop()
		refresh := time.NewTicker(time.Second)
		defer refresh.Stop()
		var lastWindow, lastTag string
		reconcile := true
		for {
			window, tag := model.current()
			if reconcile {
				updateWindows(root, settings, window)
			}
			if window != "" && (reconcile || lastWindow != window || lastTag != tag) {
				publishWindow(root, window, tag)
			}
			lastWindow, lastTag = window, tag
			reconcile = false
			select {
			case <-events:
				reconcile = true
			case <-refresh.C:
				reconcile = true
			case <-sample.C:
			case <-stop:
				return
			}
		}
	}()
	return func() {
		close(stop)
		<-done
		cancel()
		for _, name := range []string{"pano-window", "ff-window"} {
			os.Remove(filepath.Join(root, name))
		}
	}
}
