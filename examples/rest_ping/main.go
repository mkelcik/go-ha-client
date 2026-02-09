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

func main() {
	host := mustEnv("HA_HOST")
	token := mustEnv("HA_TOKEN")

	client, err := ha.NewClient(host,
		ha.WithToken(token),
		ha.WithTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Ping(ctx); err != nil {
		log.Fatalf("ping failed: %v", err)
	}

	cfg, err := client.GetConfig(ctx)
	if err != nil {
		log.Fatalf("get config failed: %v", err)
	}

	fmt.Printf("Connected to Home Assistant %s (%s)\n", cfg.Version, cfg.LocationName)
}
