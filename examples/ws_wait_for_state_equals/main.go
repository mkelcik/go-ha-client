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
	targetState   = "on"
	waitTimeout   = 120 * time.Second
)

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
	// WaitForStateEquals is safe to call from multiple goroutines on the same WS client.
	ws := client.WS()
	if err := ws.Connect(context.Background()); err != nil {
		log.Fatalf("ws connect failed: %v", err)
	}
	defer ws.Close()

	// Overall timeout for waiting on target state.
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()

	// Wait until entity reaches the exact target state.
	fmt.Printf("Waiting for %s to become %q...\n", lightEntityID, targetState)
	if err := ws.WaitForStateEquals(ctx, lightEntityID, targetState); err != nil {
		log.Fatalf("wait for state equals failed: %v", err)
	}

	fmt.Printf("Entity %s reached target state %q\n", lightEntityID, targetState)
}
