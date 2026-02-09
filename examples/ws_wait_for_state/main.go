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
	entityID := mustEnv("HA_LIGHT_ENTITY_ID")
	targetState := envOrDefault("HA_TARGET_STATE", "on")
	timeoutSeconds := envOrDefault("HA_WAIT_TIMEOUT_SECONDS", "120")

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

	duration, err := time.ParseDuration(timeoutSeconds + "s")
	if err != nil {
		log.Fatalf("invalid HA_WAIT_TIMEOUT_SECONDS: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	fmt.Printf("Waiting for %s to become %q...\n", entityID, targetState)
	err = ws.WaitForState(ctx, entityID, func(s ha.State) bool {
		return s.State == targetState
	})
	if err != nil {
		log.Fatalf("wait for state failed: %v", err)
	}

	fmt.Printf("Entity %s reached target state %q\n", entityID, targetState)
}
