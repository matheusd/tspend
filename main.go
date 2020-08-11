package main

import (
	"fmt"
	"os"
)

func _main() error {
	// Load configuration and parse command line.  This function also
	// initializes logging and configures it accordingly.
	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}

	ctx := shutdownListener()

	var mainErr error
	go func() {
		mainErr = genTspend(cfg, ctx)
		requestShutdown()
	}()

	// Wait until the app is commanded to shutdown to close the server.
	select {
	case <-ctx.Done():
	}

	return mainErr
}

func main() {
	if err := _main(); err != nil && err != errCmdDone {
		fmt.Println(err)
		os.Exit(1)
	}
}
