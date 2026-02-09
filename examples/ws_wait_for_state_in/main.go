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
	haHost        = "http://homeassistant.local:8123"
	haToken       = "YOUR_LONG_LIVED_TOKEN"
	lightEntityID = "light.kitchen"
	waitTimeout   = 120 * time.Second
)

var targetStates = []string{
	"on",
	"unavailable",
}

func main() {
	// Create base client.
	client, err := ha.NewClient(haHost,
		ha.WithToken(haToken),
		ha.WithTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	// Connect websocket and authenticate.
	// Open one WS connection and reuse it across waits.
	// WaitForStateIn is safe to call from multiple goroutines on the same WS client.
	ws := client.WS()
	if err := ws.Connect(context.Background()); err != nil {
		log.Fatalf("ws connect failed: %v", err)
	}
	defer ws.Close()

	if len(targetStates) == 0 {
		log.Fatal("add at least one target state")
	}

	// Overall timeout for waiting on one of target states.
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()

	fmt.Printf("Waiting for %s to become one of %v...\n", lightEntityID, targetStates)
	if err := ws.WaitForStateIn(ctx, lightEntityID, targetStates...); err != nil {
		log.Fatalf("wait for state in failed: %v", err)
	}

	fmt.Printf("Entity %s reached one of target states %v\n", lightEntityID, targetStates)
}
