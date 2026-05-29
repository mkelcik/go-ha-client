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
	haHost   = "http://homeassistant.local:8123"
	haToken  = "YOUR_LONG_LIVED_TOKEN"
	entityID = "light.kitchen"
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

	// List current exposure map.
	exposed, err := ws.ListExposedEntities(ctx)
	if err != nil {
		log.Fatalf("list exposed: %v", err)
	}
	fmt.Printf("current exposure for %s: %+v\n", entityID, exposed.ExposedEntities[entityID])

	// Expose entity to the conversation assistant.
	err = ws.ExposeEntity(ctx, ha.ExposeEntityRequest{
		Assistants:   []string{"conversation"},
		EntityIDs:    []string{entityID},
		ShouldExpose: true,
	})
	if err != nil {
		log.Fatalf("expose entity: %v", err)
	}
	fmt.Printf("exposed %s to conversation assistant\n", entityID)
}
