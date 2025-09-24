package main

import (
	"fmt"
	"os"

	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/di"
)

func main() {
	cfg := config.New()
	container, err := di.New(di.WithConfig(cfg))
	if err != nil {
		fmt.Fprintf(os.Stderr, "cascade: failed to initialise dependencies: %v\n", err)
		os.Exit(1)
	}
	_ = container
	fmt.Println("cascade CLI not yet implemented")
}
