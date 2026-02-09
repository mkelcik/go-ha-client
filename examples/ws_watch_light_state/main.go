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
	haHost        = "http://homeassistant.local:8123"
	haToken       = "YOUR_LONG_LIVED_TOKEN"
	lightEntityID = "light.kitchen"
	watchTimeout  = 120 * time.Second
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

	// Subscribe only to state_changed events for one entity.
	sub, err := ws.SubscribeStateChanged(context.Background(), lightEntityID)
	if err != nil {
		log.Fatalf("subscribe failed: %v", err)
	}
	defer sub.Unsubscribe(context.Background())

	// Stop watching after configured timeout.
	ctx, cancel := context.WithTimeout(context.Background(), watchTimeout)
	defer cancel()

	// Consume events/errors from subscription channels.
	fmt.Printf("Watching state changes for %s (timeout %s)\n", lightEntityID, watchTimeout)
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
			// Decode the event payload to strongly-typed state_changed data.
			data, ok, err := ev.StateChanged()
			if err != nil || !ok || data.NewState == nil {
				continue
			}
			oldState := ""
			if data.OldState != nil {
				oldState = data.OldState.State
			}
			fmt.Printf("%s: %s -> %s\n", data.EntityID, oldState, data.NewState.State)
		}
	}
}
