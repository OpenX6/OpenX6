//go:build !linux

package main

func preparePrivateSD(StagePlan, string, string, string, *stageNotice) error {
	return fail("SD staging requires target Linux")
}
func publishPrivateSD() error        { return fail("SD staging requires target Linux") }
func recoverPrivateSD(SDState) error { return nil }

func checkPrivateSD() error { return nil }

func noUSBExport() error { return fail("requires target Linux") }
