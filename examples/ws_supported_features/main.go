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

	// Opt-in: advertise support for message coalescing. Must be called
	// explicitly; Connect never sends supported_features automatically so that
	// existing integrations keep their exact handshake sequence.
	if err := ws.DeclareSupportedFeatures(ctx, map[string]interface{}{
		"coalesce_messages": 1,
	}); err != nil {
		log.Fatalf("declare supported features: %v", err)
	}
	fmt.Println("supported_features declared: coalesce_messages=1")
}
