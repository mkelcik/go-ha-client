package main

import (
	"context"
	"fmt"
	"log"
	"time"

	ha "github.com/mkelcik/go-ha-client/v2"
)

const (
	// Replace with your Home Assistant URL/token and desired template.
	haHost      = "http://homeassistant.local:8123"
	haToken     = "YOUR_LONG_LIVED_TOKEN"
	templateStr = "{{ states('sun.sun') }}"
)

func main() {
	// Create REST client.
	client, err := ha.NewClient(haHost,
		ha.WithToken(haToken),
		ha.WithTimeout(20*time.Second),
	)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	// Timeout for render request.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Ask Home Assistant to render the Jinja template.
	rendered, err := client.RenderTemplate(ctx, templateStr)
	if err != nil {
		log.Fatalf("render template failed: %v", err)
	}

	fmt.Printf("Template: %s\nResult: %s\n", templateStr, rendered)
}
