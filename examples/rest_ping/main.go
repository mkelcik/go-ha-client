package main

import (
	"context"
	"fmt"
	"log"
	"time"

	ha "github.com/mkelcik/go-ha-client/v2"
)

const (
	// Replace with your Home Assistant URL and long-lived token.
	haHost  = "http://homeassistant.local:8123"
	haToken = "YOUR_LONG_LIVED_TOKEN"
)

func main() {
	// Create REST client.
	client, err := ha.NewClient(haHost,
		ha.WithToken(haToken),
		ha.WithTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	// Shared timeout for health checks below.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Quick connectivity/auth check.
	if err := client.Ping(ctx); err != nil {
		log.Fatalf("ping failed: %v", err)
	}

	// Read basic instance info.
	cfg, err := client.GetConfig(ctx)
	if err != nil {
		log.Fatalf("get config failed: %v", err)
	}

	fmt.Printf("Connected to Home Assistant %s (%s)\n", cfg.Version, cfg.LocationName)
}
