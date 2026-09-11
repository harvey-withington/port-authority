package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"

	"portauthority/core/api"
	"portauthority/core/kb"
)

// runServe hosts the local HTTP API until interrupted.
func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:7911", "loopback address to listen on")
	fixture := fs.String("fixture", "", "replay a saved snapshot instead of reading hardware")
	defaultKB, _ := kb.DefaultLocalDir()
	kbDir := fs.String("kb-dir", defaultKB, "directory of the user's own knowledge base; empty for read-only")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := kb.UseLocalDir(*kbDir); err != nil {
		return fmt.Errorf("local knowledge base: %w", err)
	}
	p, err := selectProvider(*fixture)
	if err != nil {
		return err
	}
	logger := log.New(os.Stderr, "pactl serve: ", log.LstdFlags)
	svc := api.NewService(p, api.WithLogger(logger))

	base := "http://" + *addr + "/api/v1"
	fmt.Printf("Port Authority API (provider %s) listening on %s\n", p.Capabilities().Platform, *addr)
	for _, ep := range []string{"health", "capabilities", "topology", "insights", "devices/{id}", "throughput", "kb/docks"} {
		fmt.Printf("  GET %s/%s\n", base, ep)
	}
	if *kbDir != "" {
		fmt.Printf("  POST %s/kb/docks, DELETE %s/kb/docks/{id}  (your docks, in %s)\n", base, base, *kbDir)
	}
	fmt.Printf("  WS  ws://%s/api/v1/stream  (events: %s)\n", *addr, strings.Join(api.EventTypes, ", "))
	fmt.Println("Press Ctrl+C to stop.")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// The live side (hotplug refreshes, throughput samples) runs alongside
	// the HTTP server; a provider without those streams just logs that.
	runErr := make(chan error, 1)
	go func() {
		err := svc.Run(ctx)
		if err != nil {
			logger.Printf("live events stopped: %v", err)
		}
		runErr <- err
	}()

	err = api.ListenAndServe(ctx, *addr, svc.Handler())
	stop()
	if rerr := <-runErr; err == nil && rerr != nil {
		err = rerr
	}
	return err
}
