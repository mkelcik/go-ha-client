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
	haHost       = "http://homeassistant.local:8123"
	haToken      = "YOUR_LONG_LIVED_TOKEN"
	watchTimeout = 180 * time.Second
)

func main() {
	// Create base client.
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

	// Subscribe to call_service events.
	sub, err := ws.SubscribeEvents(context.Background(), ha.EventTypeCallService)
	if err != nil {
		log.Fatalf("subscribe call_service failed: %v", err)
	}
	defer sub.Unsubscribe(context.Background())

	// Stop watching after configured timeout.
	ctx, cancel := context.WithTimeout(context.Background(), watchTimeout)
	defer cancel()

	fmt.Printf("Watching call_service events (timeout %s)\n", watchTimeout)
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("Done: %v\n", ctx.Err())
			return
		case err := <-sub.Errors():
			if err != nil {
				log.Fatalf("subscription error: %v", err)
			}
		case ev, ok := <-sub.Events():
			if !ok {
				fmt.Println("subscription closed")
				return
			}
			// Decode event payload into typed call_service structure.
			data, ok, err := ev.CallServiceEvent()
			if err != nil || !ok {
				continue
			}
			fmt.Printf("%s.%s service_data=%v\n", data.Domain, data.Service, data.ServiceData)
		}
	}
}
