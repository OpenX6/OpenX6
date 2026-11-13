// Host-side companion for the macOS desktop.
package main

import (
	"fmt"
	"openx6.local/desktop/update"
	"os"
)

func main() {
	var result string
	var err error
	switch {
	case len(os.Args) == 2 && os.Args[1] == "check":
		result, err = update.Check()
	case len(os.Args) == 2 && os.Args[1] == "active":
		result = update.Active()
	case len(os.Args) == 3 && os.Args[1] == "ready":
		err = update.Ready(os.Args[2])
	case len(os.Args) == 3 && os.Args[1] == "install":
		result, err = update.Install(os.Args[2])
	default:
		err = fmt.Errorf("unknown desktop update command")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(result)
}
