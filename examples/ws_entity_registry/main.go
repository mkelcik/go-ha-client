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
	// Replace with your Home Assistant URL and long-lived token.
	haHost       = "http://homeassistant.local:8123"
	haToken      = "YOUR_LONG_LIVED_TOKEN"
	filterDomain = "light" // print only entities in this domain; empty = all
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

	reg, err := ws.ListEntityRegistryForDisplay(ctx)
	if err != nil {
		log.Fatalf("list for display: %v", err)
	}

	fmt.Printf("Registry entries: %d\n", len(reg.Entities))
	for _, e := range reg.Entities {
		// Home Assistant uses short keys in this payload; "ei" is entity_id.
		entityID, _ := e["ei"].(string)
		if filterDomain != "" && !strings.HasPrefix(entityID, filterDomain+".") {
			continue
		}
		fmt.Printf("  %s\n", entityID)
	}
}
