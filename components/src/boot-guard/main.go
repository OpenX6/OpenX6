// Record boot health and apply explicit one-boot maintenance requests.
package main

import (
	"fmt"
	"os"
)

func main() {
	usage := func() {
		fmt.Fprintln(os.Stderr, "usage: x6-boot-guard enter|health|status|clear | force <0..2>")
		os.Exit(64)
	}
	if len(os.Args) < 2 {
		usage()
	}
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "root required")
		os.Exit(77)
	}
	g := production()
	switch {
	case len(os.Args) == 2 && os.Args[1] == "enter":
		os.Exit(g.enter())
	case len(os.Args) == 2 && os.Args[1] == "health":
		os.Exit(g.health())
	case len(os.Args) == 2 && os.Args[1] == "status":
		os.Exit(g.status())
	case len(os.Args) == 2 && os.Args[1] == "clear":
		os.Exit(g.clear())
	case len(os.Args) == 3 && os.Args[1] == "ack-card":
		os.Exit(g.ackCard(os.Args[2]))
	case len(os.Args) == 4 && os.Args[1] == "arm-card":
		os.Exit(g.armCard(os.Args[2], os.Args[3]))
	case len(os.Args) == 3 && os.Args[1] == "force":
		os.Exit(g.force(os.Args[2]))
	default:
		usage()
	}
}
