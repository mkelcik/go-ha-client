package main

import (
	"encoding/json"
	"fmt"
	"log"

	ha "github.com/mkelcik/go-ha-client/v2"
)

type StateChangedData struct {
	EntityID string `json:"entity_id"`
	NewState struct {
		State string `json:"state"`
	} `json:"new_state"`
}

func main() {
	// This example is offline and does not connect to Home Assistant.
	// It only demonstrates how to decode event payloads into a typed Go struct.
	ev := ha.WSEvent{
		EventType: ha.EventTypeStateChanged,
		Data: json.RawMessage(`{
			"entity_id":"light.kitchen",
			"new_state":{"state":"on"}
		}`),
	}

	// Decode raw event JSON into our custom type.
	decoded, err := ha.DecodeEventData[StateChangedData](ev)
	if err != nil {
		log.Fatalf("decode event data failed: %v", err)
	}

	fmt.Printf("Decoded event helper output: %s -> %s\n", decoded.EntityID, decoded.NewState.State)
}
