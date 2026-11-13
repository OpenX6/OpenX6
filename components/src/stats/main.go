package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	replay := flag.String("replay", "", "offline trace replay")
	flag.Parse()
	if *replay != "" {
		f, e := os.Open(*replay)
		if e != nil {
			panic(e)
		}
		defer f.Close()
		s := bufio.NewScanner(f)
		m := fresh()
		for s.Scan() {
			if ev, ok := parse(s.Text()); ok {
				m.add(ev)
			}
		}
		if e = s.Err(); e != nil {
			panic(e)
		}
		r := map[uint64]any{}
		for id, v := range m.Streams {
			r[id] = map[string]any{"frames": v.Frames, "gaps": v.Gaps, "period_ns": v.Period, "mean_ms": v.SumLatency / float64(v.Returns), "invalid": v.Bad}
		}
		b, _ := json.MarshalIndent(map[string]any{"streams": r, "trace_invalid": m.Lost, "pending": len(m.Pending)}, "", "  ")
		fmt.Println(string(b))
		return
	}
	if e := live(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
