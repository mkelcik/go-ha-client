package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	ha "github.com/mkelcik/go-ha-client/v2"
)

const (
	// Replace with values from your Home Assistant instance.
	haHost         = "http://homeassistant.local:8123"
	haToken        = "YOUR_LONG_LIVED_TOKEN"
	switchEntityID = "switch.kitchen"
	switchAction   = "toggle" // on | off | toggle
)

func main() {
	// Normalize action so values like "ON" still work.
	action := strings.ToLower(switchAction)

	// Map user-friendly action to Home Assistant service name.
	service := ha.ServiceToggle
	switch action {
	case "on":
		service = ha.ServiceTurnOn
	case "off":
		service = ha.ServiceTurnOff
	case "toggle":
		service = ha.ServiceToggle
	default:
		log.Fatalf("invalid switchAction %q (supported: on, off, toggle)", action)
	}

	// Create base client used for websocket connection.
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

	// Timeout for service call over websocket.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Send service call payload.
	_, err = ws.CallService(ctx, ha.DomainSwitch, service, map[string]interface{}{
		"entity_id": switchEntityID,
	})
	if err != nil {
		log.Fatalf("ws call service failed: %v", err)
	}

	fmt.Printf("WS switch action %q sent to %s\n", service, switchEntityID)
}
