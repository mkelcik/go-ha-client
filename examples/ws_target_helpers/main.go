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
	client, err := ha.NewClient(haHost,
		ha.WithToken(haToken),
		ha.WithTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	ws := client.WS()
	if err := ws.Connect(context.Background()); err != nil {
		log.Fatalf("ws connect failed: %v", err)
	}
	defer ws.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	target := ha.TargetSelector{EntityID: []string{"light.kitchen"}}

	extracted, err := ws.ExtractFromTarget(ctx, target, false)
	if err != nil {
		log.Fatalf("extract from target: %v", err)
	}
	fmt.Printf("extracted entities: %v\n", extracted.EntityIDs)

	triggers, err := ws.GetTriggersForTarget(ctx, target, true)
	if err != nil {
		log.Fatalf("get triggers: %v", err)
	}
	fmt.Printf("triggers: %d\n", len(triggers))

	conditions, err := ws.GetConditionsForTarget(ctx, target, true)
	if err != nil {
		log.Fatalf("get conditions: %v", err)
	}
	fmt.Printf("conditions: %d\n", len(conditions))

	services, err := ws.GetServicesForTarget(ctx, target, true)
	if err != nil {
		log.Fatalf("get services: %v", err)
	}
	fmt.Printf("services: %d\n", len(services))
}
