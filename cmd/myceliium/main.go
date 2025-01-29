package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"mycelium/internal/config"
	"mycelium/internal/node"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	node, err := node.NewNode(ctx, cfg)
	if err != nil {
		log.Fatalf("Failed to create node: %v", err)
	}

	// Start the node
	if err := node.Start(); err != nil {
		log.Fatalf("Failed to start node: %v", err)
	}

	// Wait for shutdown signal
	<-sigChan
	log.Println("Shutting down...")

	// Graceful shutdown
	if err := node.Stop(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}
}
