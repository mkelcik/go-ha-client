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
	lightEntityID   = "light.kitchen"
	lightBrightness = 200 // 0-255
)

func main() {
	// Validate constant used in service payload.
	if lightBrightness < 0 || lightBrightness > 255 {
		log.Fatalf("invalid lightBrightness %d (expected 0-255)", lightBrightness)
	}

	// Create base client.
	client, err := ha.NewClient(haHost,
		ha.WithToken(haToken),
		ha.WithTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	// Create websocket client and connect once.
	ws := client.WS()
	if err := ws.Connect(context.Background()); err != nil {
		log.Fatalf("ws connect failed: %v", err)
	}
	defer ws.Close()

	// Timeout for service call roundtrip.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Call service and wait for Home Assistant result.
	result, err := ws.CallServiceForEntity(ctx,
		ha.DomainLight,
		ha.ServiceTurnOn,
		lightEntityID,
		map[string]interface{}{
			"brightness": lightBrightness,
		},
	)
	if err != nil {
		log.Fatalf("call service failed: %v", err)
	}

	fmt.Printf("Sent turn_on to %s, context id: %s\n", lightEntityID, result.Context.ID)
}
