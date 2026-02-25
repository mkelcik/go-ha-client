package main

import (
	"context"
	"fmt"
	"log"
	"time"

	ha "github.com/mkelcik/go-ha-client/v2"
)

const (
	// Replace with values from your Home Assistant instance.
	haHost          = "http://homeassistant.local:8123"
	haToken         = "YOUR_LONG_LIVED_TOKEN"
	historyHours    = 24
	historyEntityID = "light.kitchen"
)

func main() {
	// Basic validation for demo constants.
	if historyHours <= 0 {
		log.Fatalf("invalid historyHours %d", historyHours)
	}

	// Create REST client used for history API.
	client, err := ha.NewClient(haHost,
		ha.WithToken(haToken),
		ha.WithTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	// Build query: last N hours, selected entity, minimal payload.
	query := ha.NewHistoryQuery().
		WithStart(time.Now().Add(-time.Duration(historyHours) * time.Hour)).
		WithEntities(historyEntityID).
		WithMinimalResponse(true)

	// Timeout protects from hanging HTTP calls.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fetch history blocks from Home Assistant.
	history, err := client.GetHistory(ctx, query)
	if err != nil {
		log.Fatalf("get history failed: %v", err)
	}

	fmt.Printf("History blocks for %s in last %d hours: %d\n", historyEntityID, historyHours, len(history))
}
