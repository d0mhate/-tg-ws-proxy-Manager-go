//go:build minimal || no_qr

package main

import (
	"fmt"
	"os"
)

func handleQRCommand(args []string) bool {
	if len(args) == 0 || args[0] != "qr" {
		return false
	}
	fmt.Fprintln(os.Stderr, "qr support is not included in this build")
	os.Exit(1)
	return true
}
