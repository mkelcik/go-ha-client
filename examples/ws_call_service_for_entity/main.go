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
	entityID := mustEnv("HA_LIGHT_ENTITY_ID")
	brightnessRaw := envOrDefault("HA_BRIGHTNESS", "200")

	brightness, err := strconv.Atoi(brightnessRaw)
	if err != nil || brightness < 0 || brightness > 255 {
		log.Fatalf("invalid HA_BRIGHTNESS %q (expected 0-255)", brightnessRaw)
	}

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

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	result, err := ws.CallServiceForEntity(ctx,
		ha.DomainLight,
		ha.ServiceTurnOn,
		entityID,
		map[string]interface{}{
			"brightness": brightness,
		},
	)
	if err != nil {
		log.Fatalf("call service failed: %v", err)
	}

	fmt.Printf("Sent turn_on to %s, context id: %s\n", entityID, result.Context.ID)
}
