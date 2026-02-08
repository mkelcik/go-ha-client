package go_ha_client

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWSAutoReconnect(t *testing.T) {
	t.Parallel()

	// Coordinate test phases
	firstConnect := make(chan struct{})
	disconnect := make(chan struct{})
	reconnected := make(chan struct{})
	subscribed := make(chan struct{})

	errCh := make(chan error, 1)

	// Server state
	var connectionCount int32

	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		count := atomic.AddInt32(&connectionCount, 1)

		defer conn.Close()

		// Auth flow
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

		if count == 1 {
			close(firstConnect)
			// Wait for client to subscribe
			subReq := map[string]interface{}{}
			if err := conn.ReadJSON(&subReq); err != nil {
				reportHandlerErr(errCh, err)
				return
			}
			// Confirm subscription
			_ = conn.WriteJSON(map[string]interface{}{
				"id":      subReq["id"],
				"type":    "result",
				"success": true,
				"result":  nil,
			})
			close(subscribed)

			// Wait for signal to disconnect
			<-disconnect
			return // Close connection
		}

		if count == 2 {
			close(reconnected)
			// Expect subscription restore
			subReq := map[string]interface{}{}
			if err := conn.ReadJSON(&subReq); err != nil {
				reportHandlerErr(errCh, err)
				return
			}
			if subReq["type"] != "subscribe_events" {
				reportHandlerErr(errCh, errors.New("expected subscription restore"))
				return
			}
			// Confirm restore
			_ = conn.WriteJSON(map[string]interface{}{
				"id":      subReq["id"],
				"type":    "result",
				"success": true,
				"result":  nil,
			})

			// Send event to prove it works
			_ = conn.WriteJSON(map[string]interface{}{
				"id":   subReq["id"],
				"type": "event",
				"event": map[string]interface{}{
					"event_type": "state_changed",
					"data": map[string]interface{}{
						"entity_id": "light.reconnect",
						"new_state": map[string]interface{}{"state": "on"},
					},
				},
			})

			// Keep connection open
			time.Sleep(1 * time.Second)
		}
	})
	defer srv.Close()

	// Client with auto-reconnect
	// Use small backoff for test speed
	client, err := NewClient(srv.URL, WithToken("token"))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ws := client.WS(
		WithAutoReconnect(true),
		WithReconnectBackoff(10*time.Millisecond, 100*time.Millisecond),
	)

	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer ws.Close()

	// 1. Subscribe
	sub, err := ws.SubscribeEvents(context.Background(), "state_changed")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	<-firstConnect
	<-subscribed

	// 2. Trigger disconnect
	close(disconnect)

	// 3. Wait for reconnect
	select {
	case <-reconnected:
		// success
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for reconnect")
	}

	// 4. Verify subscription works after reconnect
	select {
	case ev := <-sub.Events():
		if ev.EventType != "state_changed" {
			t.Errorf("unexpected event type: %s", ev.EventType)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("timeout waiting for event after reconnect")
	}

	assertNoHandlerErr(t, errCh)
}
