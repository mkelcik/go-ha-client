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

	// Valid state trigger.
	res, err := ws.ValidateConfig(ctx, ha.ValidateConfigRequest{
		Trigger: map[string]interface{}{
			"platform":  "state",
			"entity_id": "light.kitchen",
		},
	})
	if err != nil {
		log.Fatalf("validate valid trigger: %v", err)
	}
	fmt.Printf("valid trigger result: %+v\n", res.Trigger)

	// Intentionally invalid action (missing service).
	res, err = ws.ValidateConfig(ctx, ha.ValidateConfigRequest{
		Action: map[string]interface{}{"service": ""},
	})
	if err != nil {
		log.Fatalf("validate invalid action: %v", err)
	}
	fmt.Printf("invalid action result: %+v\n", res.Action)
}
