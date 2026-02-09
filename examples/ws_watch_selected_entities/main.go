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

var watchedEntities = []string{
	"light.kitchen",
	"switch.garage",
}

func main() {
	if len(watchedEntities) == 0 {
		log.Fatal("add at least one entity to watchedEntities")
	}

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

	// Subscribe once and filter state_changed events for selected entities.
	sub, err := ws.SubscribeStateChangedMany(context.Background(), watchedEntities...)
	if err != nil {
		log.Fatalf("subscribe state_changed many failed: %v", err)
	}
	defer sub.Unsubscribe(context.Background())

	// Stop watching after configured timeout.
	ctx, cancel := context.WithTimeout(context.Background(), watchTimeout)
	defer cancel()

	fmt.Printf("Watching state changes for %v (timeout %s)\n", watchedEntities, watchTimeout)
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
