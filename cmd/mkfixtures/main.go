package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: mkfixtures variants GOOD.WEBM OUTDIR | png-frames OUTDIR")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "variants":
		if len(os.Args) != 4 {
			err = fmt.Errorf("usage: mkfixtures variants GOOD.WEBM OUTDIR")
		} else {
			err = writeVariants(os.Args[2], os.Args[3])
		}
	case "png-frames":
		if len(os.Args) != 3 {
			err = fmt.Errorf("usage: mkfixtures png-frames OUTDIR")
		} else {
			err = writePNGFrames(os.Args[2])
		}
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkfixtures:", err)
		os.Exit(1)
	}
}
