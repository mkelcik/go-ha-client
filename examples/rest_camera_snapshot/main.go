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

const (
	// Replace with values from your Home Assistant instance.
	haHost         = "http://homeassistant.local:8123"
	haToken        = "YOUR_LONG_LIVED_TOKEN"
	cameraEntityID = "camera.front_door"
	outputFile     = "camera.jpg"
)

func main() {
	// Create REST client used for camera API calls.
	client, err := ha.NewClient(haHost,
		ha.WithToken(haToken),
		ha.WithTimeout(45*time.Second),
	)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	// Keep the whole request bounded by timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Download JPEG frame from camera entity.
	img, err := client.GetCameraJpeg(ctx, cameraEntityID)
	if err != nil {
		log.Fatalf("get camera jpeg failed: %v", err)
	}

	// Save image to local file.
	f, err := os.Create(outputFile)
	if err != nil {
		log.Fatalf("create output file failed: %v", err)
	}
	defer f.Close()

	// Encode image.Image into JPEG bytes.
	if err := jpeg.Encode(f, img, nil); err != nil {
		log.Fatalf("encode jpeg failed: %v", err)
	}

	fmt.Printf("Saved camera snapshot to %s\n", outputFile)
}
