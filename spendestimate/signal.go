// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
)

// shutdownRequestChannel is used to initiate shutdown from one of the
// subsystems using the same code paths as when an interrupt signal is received.
var shutdownRequestChannel = make(chan struct{})

// interruptSignals defines the default signals to catch in order to do a proper
// shutdown.  This may be modified during init depending on the platform.
var interruptSignals = []os.Signal{os.Interrupt}

// requestShutdown requests the process to end. It blocks until the shutdown
// process signal is received.
func requestShutdown() {
	shutdownRequestChannel <- struct{}{}
}

// shutdownListener listens for OS Signals such as SIGINT (Ctrl+C) and shutdown
// requests from shutdownRequestChannel.  It returns a context that is canceled
// when either signal is received.
func shutdownListener() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		interruptChannel := make(chan os.Signal, 1)
		signal.Notify(interruptChannel, interruptSignals...)

		// Listen for initial shutdown signal and cancel the returned context.
		select {
		case sig := <-interruptChannel:
			fmt.Fprintf(os.Stderr, "Received signal (%s).  Shutting down...\n", sig)

		case <-shutdownRequestChannel:
			fmt.Fprintf(os.Stderr, "Shutdown requested.  Shutting down...\n")
		}
		cancel()

		// Listen for repeated signals and display a message so the user
		// knows the shutdown is in progress and the process is not
		// hung.
		for {
			select {
			case sig := <-interruptChannel:
				fmt.Fprintf(os.Stderr, "Received signal (%s).  Already "+
					"shutting down...\n", sig)

			case <-shutdownRequestChannel:
				fmt.Fprintf(os.Stderr, "Shutdown requested.  Already "+
					"shutting down...\n")
			}
		}
	}()

	return ctx
}

// shutdownRequested returns true when the context returned by shutdownListener
// was canceled.  This simplifies early shutdown slightly since the caller can
// just use an if statement instead of a select.
func shutdownRequested(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
	}

	return false
}
