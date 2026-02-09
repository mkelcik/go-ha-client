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
	template := envOrDefault("HA_TEMPLATE", "{{ states('sun.sun') }}")

	client, err := ha.NewClient(host,
		ha.WithToken(token),
		ha.WithTimeout(20*time.Second),
	)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	rendered, err := client.RenderTemplate(ctx, template)
	if err != nil {
		log.Fatalf("render template failed: %v", err)
	}

	fmt.Printf("Template: %s\nResult: %s\n", template, rendered)
}
