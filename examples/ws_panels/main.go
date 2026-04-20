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

	panels, err := ws.GetPanels(ctx)
	if err != nil {
		log.Fatalf("get panels failed: %v", err)
	}

	fmt.Printf("Registered panels: %d\n", len(panels))
	for urlPath, panel := range panels {
		fmt.Printf("  %-20s component=%-20s admin=%t icon=%s\n",
			urlPath, panel.ComponentName, panel.RequireAdmin, panel.Icon)
	}
}
