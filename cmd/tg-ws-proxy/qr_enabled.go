//go:build !minimal && !no_qr

package main

import (
	"fmt"
	"os"

	"rsc.io/qr"
)

func handleQRCommand(args []string) bool {
	if len(args) == 0 || args[0] != "qr" {
		return false
	}
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: tg-ws-proxy qr <link>")
		os.Exit(1)
	}
	code, err := qr.Encode(args[1], qr.L)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	size := code.Size
	blk := func(x, y int) bool {
		return x >= 0 && x < size && y >= 0 && y < size && code.Black(x, y)
	}
	for y := -2; y < size+2; y += 2 {
		for x := -2; x < size+2; x++ {
			t, b := blk(x, y), blk(x, y+1)
			switch {
			case t && b:
				fmt.Fprint(os.Stdout, "█")
			case t:
				fmt.Fprint(os.Stdout, "▀")
			case b:
				fmt.Fprint(os.Stdout, "▄")
			default:
				fmt.Fprint(os.Stdout, " ")
			}
		}
		fmt.Fprintln(os.Stdout)
	}
	return true
}
