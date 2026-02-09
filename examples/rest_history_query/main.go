package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
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
	entityID := mustEnv("HA_ENTITY_ID")

	hoursRaw := envOrDefault("HA_HISTORY_HOURS", "24")
	hours, err := strconv.Atoi(hoursRaw)
	if err != nil || hours <= 0 {
		log.Fatalf("invalid HA_HISTORY_HOURS %q", hoursRaw)
	}

	client, err := ha.NewClient(host,
		ha.WithToken(token),
		ha.WithTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	query := ha.NewHistoryQuery().
		WithStart(time.Now().Add(-time.Duration(hours) * time.Hour)).
		WithEntities(entityID).
		WithMinimalResponse(true)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	history, err := client.GetHistory(ctx, query)
	if err != nil {
		log.Fatalf("get history failed: %v", err)
	}

	fmt.Printf("History blocks for %s in last %d hours: %d\n", entityID, hours, len(history))
}
