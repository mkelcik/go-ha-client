package main

import (
	"context"
	"fmt"
	"image/jpeg"
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
	cameraEntityID := mustEnv("HA_CAMERA_ENTITY_ID")
	output := envOrDefault("HA_CAMERA_OUTPUT", "camera.jpg")

	client, err := ha.NewClient(host,
		ha.WithToken(token),
		ha.WithTimeout(45*time.Second),
	)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	img, err := client.GetCameraJpeg(ctx, cameraEntityID)
	if err != nil {
		log.Fatalf("get camera jpeg failed: %v", err)
	}

	f, err := os.Create(output)
	if err != nil {
		log.Fatalf("create output file failed: %v", err)
	}
	defer f.Close()

	if err := jpeg.Encode(f, img, nil); err != nil {
		log.Fatalf("encode jpeg failed: %v", err)
	}

	fmt.Printf("Saved camera snapshot to %s\n", output)
}
