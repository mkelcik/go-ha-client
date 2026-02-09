package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	ha "github.com/mkelcik/go-ha-client/v2"
)

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required environment variable %s", key)
	}
	return v
}

func envOrDefault(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func main() {
	host := mustEnv("HA_HOST")
	token := mustEnv("HA_TOKEN")
	lightEntityID := mustEnv("HA_LIGHT_ENTITY_ID")
	timeoutSeconds := envOrDefault("HA_WATCH_TIMEOUT_SECONDS", "120")

	client, err := ha.NewClient(host,
		ha.WithToken(token),
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

	sub, err := ws.SubscribeStateChanged(context.Background(), lightEntityID)
	if err != nil {
		log.Fatalf("subscribe failed: %v", err)
	}
	defer sub.Unsubscribe(context.Background())

	duration, err := time.ParseDuration(timeoutSeconds + "s")
	if err != nil {
		log.Fatalf("invalid HA_WATCH_TIMEOUT_SECONDS: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	fmt.Printf("Watching state changes for %s (timeout %s)\n", lightEntityID, duration)
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
