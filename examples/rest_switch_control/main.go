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

	// Create REST client.
	client, err := ha.NewClient(haHost,
		ha.WithToken(haToken),
		ha.WithTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	// Timeout for a single service call.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Call switch service via REST API.
	_, err = client.CallServiceForEntity(ctx, ha.DomainSwitch, service, switchEntityID, nil)
	if err != nil {
		log.Fatalf("call service failed: %v", err)
	}

	fmt.Printf("Switch action %q sent to %s\n", service, switchEntityID)
}
