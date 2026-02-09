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

func TestWSFailPendingOnDisconnect(t *testing.T) {
	t.Parallel()

	disconnect := make(chan struct{})
	requestReceived := make(chan struct{})

	errCh := make(chan error, 1)

	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()

		// Auth flow
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

		// Wait for request
		req := map[string]interface{}{}
		if err := conn.ReadJSON(&req); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		close(requestReceived)

		// Wait for signal to disconnect WITHOUT sending response
		<-disconnect
	})
	defer srv.Close()

	client, err := NewClient(srv.URL, WithToken("token"))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ws := client.WS(
		WithAutoReconnect(true),
		// Fast reconnect to trigger loop quickly
		WithReconnectBackoff(10*time.Millisecond, 100*time.Millisecond),
	)

	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer ws.Close()

	// Start request in background
	errResult := make(chan error, 1)
	go func() {
		_, err := ws.CallService(context.Background(), "light", "turn_on", nil)
		errResult <- err
	}()

	<-requestReceived
	// Trigger disconnect
	close(disconnect)

	// Verify request fails quickly
	select {
	case err := <-errResult:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrWSClosed) {
			t.Errorf("expected ErrWSClosed, got: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for pending request failure")
	}

	assertNoHandlerErr(t, errCh)
}

func TestWSResubscribeWithSameID(t *testing.T) {
	t.Parallel()
	testResubscribe(t, true)
}

func TestWSResubscribeWithNewID(t *testing.T) {
	t.Parallel()
	testResubscribe(t, false)
}

func TestWSReconnectSubscribeAndCallServiceEndToEnd(t *testing.T) {
	t.Parallel()

	firstSubscribed := make(chan struct{})
	firstCallDone := make(chan struct{})
	restored := make(chan struct{})
	reconnected := make(chan struct{})

	errCh := make(chan error, 1)
	var connectionCount int32
	const initialSubID int64 = 10
	const restoredSubID int64 = 20

	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		count := atomic.AddInt32(&connectionCount, 1)
		defer conn.Close()

		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

		if count == 1 {
			subReq := map[string]interface{}{}
			if err := conn.ReadJSON(&subReq); err != nil {
				reportHandlerErr(errCh, err)
				return
			}
			if subReq["type"] != "subscribe_events" {
				reportHandlerErr(errCh, errors.New("expected subscribe_events"))
				return
			}
			_ = conn.WriteJSON(map[string]interface{}{
				"id":      subReq["id"],
				"type":    "result",
				"success": true,
				"result":  initialSubID,
			})
			close(firstSubscribed)

			callReq := map[string]interface{}{}
			if err := conn.ReadJSON(&callReq); err != nil {
				reportHandlerErr(errCh, err)
				return
			}
			if callReq["type"] != "call_service" {
				reportHandlerErr(errCh, errors.New("expected call_service"))
				return
			}
			_ = conn.WriteJSON(map[string]interface{}{
				"id":      callReq["id"],
				"type":    "result",
				"success": true,
				"result": map[string]interface{}{
					"context": map[string]interface{}{
						"id": "ctx-1",
					},
				},
			})
			close(firstCallDone)
			return
		}

		if count == 2 {
			close(reconnected)

			subReq := map[string]interface{}{}
			if err := conn.ReadJSON(&subReq); err != nil {
				reportHandlerErr(errCh, err)
				return
			}
			if subReq["type"] != "subscribe_events" {
				reportHandlerErr(errCh, errors.New("expected subscribe_events restore"))
				return
			}
			_ = conn.WriteJSON(map[string]interface{}{
				"id":      subReq["id"],
				"type":    "result",
				"success": true,
				"result":  restoredSubID,
			})
			close(restored)

			_ = conn.WriteJSON(map[string]interface{}{
				"id":   restoredSubID,
				"type": "event",
				"event": map[string]interface{}{
					"event_type": "state_changed",
					"data": map[string]interface{}{
						"entity_id": "light.office",
					},
				},
			})

			callReq := map[string]interface{}{}
			if err := conn.ReadJSON(&callReq); err != nil {
				reportHandlerErr(errCh, err)
				return
			}
			if callReq["type"] != "call_service" {
				reportHandlerErr(errCh, errors.New("expected call_service after reconnect"))
				return
			}
			_ = conn.WriteJSON(map[string]interface{}{
				"id":      callReq["id"],
				"type":    "result",
				"success": true,
				"result": map[string]interface{}{
					"context": map[string]interface{}{
						"id": "ctx-2",
					},
				},
			})
			time.Sleep(100 * time.Millisecond)
		}
	})
	defer srv.Close()

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

	sub, err := ws.SubscribeEvents(context.Background(), "state_changed")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if sub.ID() != initialSubID {
		t.Fatalf("unexpected initial subscription id: %d", sub.ID())
	}

	<-firstSubscribed

	result, err := ws.CallService(context.Background(), "light", "turn_on", nil)
	if err != nil {
		t.Fatalf("call service: %v", err)
	}
	if result.Context.ID != "ctx-1" {
		t.Fatalf("unexpected call_service context: %s", result.Context.ID)
	}
	<-firstCallDone

	select {
	case <-reconnected:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for reconnect")
	}

	select {
	case <-restored:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for subscription restore")
	}

	select {
	case ev := <-sub.Events():
		if ev.EventType != "state_changed" {
			t.Fatalf("unexpected event type: %s", ev.EventType)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for event after reconnect")
	}

	result, err = ws.CallService(context.Background(), "light", "turn_off", nil)
	if err != nil {
		t.Fatalf("call service after reconnect: %v", err)
	}
	if result.Context.ID != "ctx-2" {
		t.Fatalf("unexpected call_service context after reconnect: %s", result.Context.ID)
	}

	if sub.ID() != restoredSubID {
		t.Fatalf("unexpected restored subscription id: %d", sub.ID())
	}

	assertNoHandlerErr(t, errCh)
}

func testResubscribe(t *testing.T, sameID bool) {
	// phases
	firstConnect := make(chan struct{})
	reconnected := make(chan struct{})
	subscribed := make(chan struct{})

	errCh := make(chan error, 1)
	var connectionCount int32
	const initialSubID int64 = 50
	const restoredSubID int64 = 51
	expectedRestoredID := initialSubID
	if !sameID {
		expectedRestoredID = restoredSubID
	}

	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		count := atomic.AddInt32(&connectionCount, 1)
		defer conn.Close()

		// Auth
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

		if count == 1 {
			close(firstConnect)
			// Wait for subscribe
			subReq := map[string]interface{}{}
			if err := conn.ReadJSON(&subReq); err != nil {
				return
			}
			// Respond
			_ = conn.WriteJSON(map[string]interface{}{
				"id":      subReq["id"],
				"type":    "result",
				"success": true,
				"result":  initialSubID,
			})
			close(subscribed)

			// Wait a bit then close to simulate disconnect
			time.Sleep(100 * time.Millisecond)
			return
		}

		if count == 2 {
			close(reconnected)
			// Wait for restore subscribe
			subReq := map[string]interface{}{}
			if err := conn.ReadJSON(&subReq); err != nil {
				return
			}
			responseID := subReq["id"]

			_ = conn.WriteJSON(map[string]interface{}{
				"id":      responseID,
				"type":    "result",
				"success": true,
				"result":  expectedRestoredID,
			})

			// Send event with that ID
			_ = conn.WriteJSON(map[string]interface{}{
				"id":   expectedRestoredID, // This matches the restored subscription ID
				"type": "event",
				"event": map[string]interface{}{
					"event_type": "state_changed",
				},
			})

			// Keep open
			time.Sleep(1 * time.Second)
		}
	})
	defer srv.Close()

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

	sub, err := ws.SubscribeEvents(context.Background(), "state_changed")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if sub.ID() != initialSubID {
		t.Fatalf("unexpected initial subscription id: %d", sub.ID())
	}

	<-firstConnect
	<-subscribed

	// Wait for reconnect
	select {
	case <-reconnected:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for reconnect")
	}

	// Wait for event
	select {
	case <-sub.Events():
		// Success
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for event after resubscribe")
	}
	if sub.ID() != expectedRestoredID {
		t.Fatalf("unexpected restored subscription id: %d", sub.ID())
	}

	assertNoHandlerErr(t, errCh)
}
