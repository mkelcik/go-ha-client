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
	ws := client.WS()
	if err := ws.Connect(context.Background()); err != nil {
		log.Fatalf("ws connect failed: %v", err)
	}
	defer ws.Close()

	// Overall timeout for waiting on target state.
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()

	// Wait until callback returns true for a new state.
	fmt.Printf("Waiting for %s to become %q...\n", lightEntityID, targetState)
	err = ws.WaitForState(ctx, lightEntityID, func(s ha.State) bool {
		return s.State == targetState
	})
	if err != nil {
		log.Fatalf("wait for state failed: %v", err)
	}

	fmt.Printf("Entity %s reached target state %q\n", lightEntityID, targetState)
}
