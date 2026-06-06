//go:build windows

package main

import "os"

func watchSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
