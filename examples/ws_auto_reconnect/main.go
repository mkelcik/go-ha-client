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
	eventType := envOrDefault("HA_EVENT_TYPE", ha.EventTypeStateChanged)

	client, err := ha.NewClient(host,
		ha.WithToken(token),
		ha.WithTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

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

	if err := ws.Connect(context.Background()); err != nil {
		log.Fatalf("ws connect failed: %v", err)
	}
	defer ws.Close()

	sub, err := ws.SubscribeEvents(context.Background(), eventType)
	if err != nil {
		log.Fatalf("subscribe failed: %v", err)
	}
	defer sub.Unsubscribe(context.Background())

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
