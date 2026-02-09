package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
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
	entityID := mustEnv("HA_SWITCH_ENTITY_ID")
	action := strings.ToLower(envOrDefault("HA_SWITCH_ACTION", "toggle"))

	service := ha.ServiceToggle
	switch action {
	case "on":
		service = ha.ServiceTurnOn
	case "off":
		service = ha.ServiceTurnOff
	case "toggle":
		service = ha.ServiceToggle
	default:
		log.Fatalf("invalid HA_SWITCH_ACTION %q (supported: on, off, toggle)", action)
	}

	client, err := ha.NewClient(host,
		ha.WithToken(token),
		ha.WithTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	_, err = client.CallService(ctx, ha.DefaultServiceCmd{
		Domain:   ha.DomainSwitch,
		Service:  service,
		EntityID: entityID,
	})
	if err != nil {
		log.Fatalf("call service failed: %v", err)
	}

	fmt.Printf("Switch action %q sent to %s\n", service, entityID)
}
