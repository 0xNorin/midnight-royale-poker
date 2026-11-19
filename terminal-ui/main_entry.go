//go:build !mrssh && !websocket

// Desktop client entrypoint. It is mutually exclusive with the SSH guest
// server entrypoint (mrssh_main.go) via the mrssh build tag; the shared
// desktop logic stays in main.go (untagged).
package main

import (
	"fmt"
	"os"
	"runtime/debug"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			fmt.Fprintf(os.Stderr, "panic: %v\n%s\n", r, stack)
			if diag != nil {
				diag.Errorf("panic: %v\n%s", r, stack)
			}
			os.Exit(2)
		}
	}()
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "Midnight Royale:", e)
		if diag != nil {
			diag.Errorf("%v", e)
		}
		os.Exit(1)
	}
}
