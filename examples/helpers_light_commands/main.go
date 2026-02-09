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
	// Replace these values with your Home Assistant setup.
	haHost        = "http://homeassistant.local:8123"
	haToken       = "YOUR_LONG_LIVED_TOKEN"
	lightObjectID = "kitchen"
	lightAction   = "toggle" // on | off | toggle
)

func main() {
	// Normalize action so values like "ON" or "On" still work.
	action := strings.ToLower(lightAction)

	// Build and validate entity id using helper functions.
	entityID := ha.BuildEntityID(ha.DomainLight, lightObjectID)
	domain, parsedObjectID, err := ha.ParseEntityID(entityID)
	if err != nil {
		log.Fatalf("parse entity id failed: %v", err)
	}

	// Pick one of the ready-made light service commands.
	var cmd ha.DefaultServiceCmd
	switch action {
	case "on":
		cmd = ha.NewTurnLightOnCmd(entityID)
	case "off":
		cmd = ha.NewTurnLightOffCmd(entityID)
	case "toggle":
		cmd = ha.NewToggleLightCmd(entityID)
	default:
		log.Fatalf("invalid lightAction %q (supported: on, off, toggle)", action)
	}

	// Create REST client with auth token and request timeout.
	client, err := ha.NewClient(haHost,
		ha.WithToken(haToken),
		ha.WithTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	// Bound this example call so it does not wait forever.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Execute selected light command.
	_, err = client.CallService(ctx, cmd)
	if err != nil {
		log.Fatalf("call service failed: %v", err)
	}

	fmt.Printf("Sent %q to %s (domain=%s object_id=%s)\n", cmd.Service, entityID, domain, parsedObjectID)
	fmt.Printf("Helper NewServiceDataEntityID output: %#v\n", ha.NewServiceDataEntityID(entityID))
}
