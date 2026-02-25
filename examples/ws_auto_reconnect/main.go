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
	haHost    = "http://homeassistant.local:8123"
	haToken   = "YOUR_LONG_LIVED_TOKEN"
	eventType = ha.EventTypeStateChanged
)

func main() {
	// Base HTTP client used by websocket client as well.
	client, err := ha.NewClient(haHost,
		ha.WithToken(haToken),
		ha.WithTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	// Enable reconnect strategy and optional reconnect callbacks.
	ws := client.WS(
		ha.WithAutoReconnect(true),
		ha.WithMaxRetries(0),
		ha.WithReconnectBackoff(time.Second, 30*time.Second),
		ha.WithOnReconnect(func() {
			fmt.Println("Reconnected")
		}),
		ha.WithOnReconnectError(func(err error) {
			fmt.Printf("Reconnect attempt failed: %v\n", err)
		}),
	)

	// Open websocket and perform Home Assistant auth handshake.
	if err := ws.Connect(context.Background()); err != nil {
		log.Fatalf("ws connect failed: %v", err)
	}
	defer ws.Close()

	// Subscribe to selected event type.
	sub, err := ws.SubscribeEvents(context.Background(), eventType)
	if err != nil {
		log.Fatalf("subscribe failed: %v", err)
	}
	defer sub.Unsubscribe(context.Background())

	// Read events until subscription is closed.
	fmt.Printf("Listening for %q events with auto reconnect enabled...\n", eventType)
	for {
		select {
		case err := <-sub.Errors():
			if err != nil {
				fmt.Printf("subscription error: %v\n", err)
			}
		case ev, ok := <-sub.Events():
			if !ok {
				fmt.Println("subscription closed")
				return
			}
			fmt.Printf("event: %s\n", ev.EventType)
		}
	}
}
