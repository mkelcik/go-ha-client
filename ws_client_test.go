package go_ha_client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func newTestWSClient(t *testing.T, serverURL, token string) *WSClient {
	t.Helper()
	client, err := NewClient(serverURL, WithToken(token), WithHTTPClient(&http.Client{}))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client.WS()
}

func TestWSConnectAndPing(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()

		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})

		authReq := map[string]interface{}{}
		if err := conn.ReadJSON(&authReq); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		if authReq["type"] != "auth" {
			reportHandlerErr(errCh, errors.New("expected auth request"))
			return
		}
		if authReq["access_token"] != "test-token" {
			reportHandlerErr(errCh, errors.New("unexpected token"))
			return
		}

		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

		msg := map[string]interface{}{}
		if err := conn.ReadJSON(&msg); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		if msg["type"] != "ping" {
			reportHandlerErr(errCh, errors.New("expected ping request"))
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{
			"type": "pong",
			"id":   msg["id"],
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	if err := ws.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSCloseDuringConnectHandshake(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	authReceived := make(chan struct{})
	releaseAuth := make(chan struct{})

	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()

		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})

		authReq := map[string]interface{}{}
		if err := conn.ReadJSON(&authReq); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		close(authReceived)

		<-releaseAuth
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")

	connectErrCh := make(chan error, 1)
	go func() {
		connectErrCh <- ws.Connect(context.Background())
	}()

	select {
	case <-authReceived:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for auth request")
	}

	if err := ws.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	close(releaseAuth)

	select {
	case err := <-connectErrCh:
		if !errors.Is(err, ErrWSClosed) {
			t.Fatalf("expected ErrWSClosed from connect, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for connect to finish")
	}

	if ws.IsConnected() {
		t.Fatalf("expected disconnected client after close")
	}
	if err := ws.Connect(context.Background()); !errors.Is(err, ErrWSClosed) {
		t.Fatalf("expected ErrWSClosed on reconnect attempt, got: %v", err)
	}

	assertNoHandlerErr(t, errCh)
}

func TestWSConnectConcurrentCallsSingleHandshake(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	authReceived := make(chan struct{})
	releaseAuth := make(chan struct{})
	var authOnce sync.Once
	var connCount int32

	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()
		atomic.AddInt32(&connCount, 1)

		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		authReq := map[string]interface{}{}
		if err := conn.ReadJSON(&authReq); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		authOnce.Do(func() {
			close(authReceived)
		})

		<-releaseAuth
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})
		time.Sleep(100 * time.Millisecond)
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	t.Cleanup(func() { _ = ws.Close() })

	const workers = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			errs <- ws.Connect(ctx)
		}()
	}

	close(start)
	select {
	case <-authReceived:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for first auth request")
	}
	close(releaseAuth)

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("connect returned error: %v", err)
		}
	}

	if got := atomic.LoadInt32(&connCount); got != 1 {
		t.Fatalf("expected exactly one handshake connection, got %d", got)
	}
	if !ws.IsConnected() {
		t.Fatalf("expected connected websocket client")
	}

	assertNoHandlerErr(t, errCh)
}

func TestWSSubscribeEvents(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()

		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

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
			"result":  nil,
		})
		_ = conn.WriteJSON(map[string]interface{}{
			"id":   subReq["id"],
			"type": "event",
			"event": map[string]interface{}{
				"event_type": "state_changed",
				"data": map[string]interface{}{
					"entity_id": "light.kitchen",
					"new_state": map[string]interface{}{"state": "on"},
				},
			},
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	sub, err := ws.SubscribeEvents(context.Background(), "state_changed")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	select {
	case ev := <-sub.Events():
		if ev.EventType != "state_changed" {
			t.Fatalf("unexpected event type: %s", ev.EventType)
		}
		if !strings.Contains(string(ev.Data), `"entity_id":"light.kitchen"`) {
			t.Fatalf("unexpected event data: %s", string(ev.Data))
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for ws event")
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSSubscribeEventsUsesResultID(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	const subscriptionID int64 = 42
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()

		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

		subReq := map[string]interface{}{}
		if err := conn.ReadJSON(&subReq); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{
			"id":      subReq["id"],
			"type":    "result",
			"success": true,
			"result":  subscriptionID,
		})
		_ = conn.WriteJSON(map[string]interface{}{
			"id":   subscriptionID,
			"type": "event",
			"event": map[string]interface{}{
				"event_type": "state_changed",
				"data": map[string]interface{}{
					"entity_id": "light.kitchen",
					"new_state": map[string]interface{}{"state": "on"},
				},
			},
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	sub, err := ws.SubscribeEvents(context.Background(), "state_changed")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if sub.ID() != subscriptionID {
		t.Fatalf("unexpected subscription id: %d", sub.ID())
	}

	select {
	case ev := <-sub.Events():
		if ev.EventType != "state_changed" {
			t.Fatalf("unexpected event type: %s", ev.EventType)
		}
		if !strings.Contains(string(ev.Data), `"entity_id":"light.kitchen"`) {
			t.Fatalf("unexpected event data: %s", string(ev.Data))
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for ws event")
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSSubscribeStateChangedFiltersEntity(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()

		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

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
			"result":  nil,
		})

		_ = conn.WriteJSON(map[string]interface{}{
			"id":   subReq["id"],
			"type": "event",
			"event": map[string]interface{}{
				"event_type": "state_changed",
				"data": map[string]interface{}{
					"entity_id": "light.other",
					"new_state": map[string]interface{}{"state": "on"},
				},
			},
		})
		_ = conn.WriteJSON(map[string]interface{}{
			"id":   subReq["id"],
			"type": "event",
			"event": map[string]interface{}{
				"event_type": "state_changed",
				"data": map[string]interface{}{
					"entity_id": "light.kitchen",
					"new_state": map[string]interface{}{"state": "on"},
				},
			},
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	sub, err := ws.SubscribeStateChanged(context.Background(), "light.kitchen")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	select {
	case ev := <-sub.Events():
		data, ok, err := ev.StateChanged()
		if err != nil {
			t.Fatalf("decode event: %v", err)
		}
		if !ok {
			t.Fatalf("expected state_changed data")
		}
		if data.EntityID != "light.kitchen" {
			t.Fatalf("unexpected entity id: %s", data.EntityID)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for ws event")
	}

	select {
	case ev, ok := <-sub.Events():
		if ok {
			t.Fatalf("unexpected extra event: %v", ev)
		}
	case <-time.After(100 * time.Millisecond):
	}

	assertNoHandlerErr(t, errCh)
}

func TestWSSubscribeStateChangedManyFiltersEntities(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()

		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

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
			"result":  nil,
		})

		_ = conn.WriteJSON(map[string]interface{}{
			"id":   subReq["id"],
			"type": "event",
			"event": map[string]interface{}{
				"event_type": "state_changed",
				"data": map[string]interface{}{
					"entity_id": "light.other",
					"new_state": map[string]interface{}{"state": "on"},
				},
			},
		})
		_ = conn.WriteJSON(map[string]interface{}{
			"id":   subReq["id"],
			"type": "event",
			"event": map[string]interface{}{
				"event_type": "state_changed",
				"data": map[string]interface{}{
					"entity_id": "light.kitchen",
					"new_state": map[string]interface{}{"state": "on"},
				},
			},
		})
		_ = conn.WriteJSON(map[string]interface{}{
			"id":   subReq["id"],
			"type": "event",
			"event": map[string]interface{}{
				"event_type": "state_changed",
				"data": map[string]interface{}{
					"entity_id": "switch.garage",
					"new_state": map[string]interface{}{"state": "off"},
				},
			},
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	sub, err := ws.SubscribeStateChangedMany(context.Background(), "light.kitchen", "switch.garage")
	if err != nil {
		t.Fatalf("subscribe many: %v", err)
	}

	got := make(map[string]bool, 2)
	for len(got) < 2 {
		select {
		case ev, ok := <-sub.Events():
			if !ok {
				t.Fatalf("subscription closed before receiving all events")
			}
			data, ok, err := ev.StateChanged()
			if err != nil {
				t.Fatalf("decode event: %v", err)
			}
			if !ok {
				t.Fatalf("expected state_changed data")
			}
			got[data.EntityID] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for filtered events")
		}
	}

	if !got["light.kitchen"] || !got["switch.garage"] {
		t.Fatalf("unexpected filtered entities: %#v", got)
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSSubscribeStateChangedManyRequiresEntities(t *testing.T) {
	t.Parallel()

	ws := &WSClient{}
	if _, err := ws.SubscribeStateChangedMany(context.Background()); !errors.Is(err, ErrEmptyEntityID) {
		t.Fatalf("expected ErrEmptyEntityID, got: %v", err)
	}
	if _, err := ws.SubscribeStateChangedMany(context.Background(), "light.kitchen", ""); !errors.Is(err, ErrEmptyEntityID) {
		t.Fatalf("expected ErrEmptyEntityID, got: %v", err)
	}
}

func TestWSWaitForState(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()

		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

		subReq := map[string]interface{}{}
		if err := conn.ReadJSON(&subReq); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{
			"id":      subReq["id"],
			"type":    "result",
			"success": true,
			"result":  nil,
		})

		_ = conn.WriteJSON(map[string]interface{}{
			"id":   subReq["id"],
			"type": "event",
			"event": map[string]interface{}{
				"event_type": "state_changed",
				"data": map[string]interface{}{
					"entity_id": "light.kitchen",
					"new_state": map[string]interface{}{"state": "off"},
				},
			},
		})
		_ = conn.WriteJSON(map[string]interface{}{
			"id":   subReq["id"],
			"type": "event",
			"event": map[string]interface{}{
				"event_type": "state_changed",
				"data": map[string]interface{}{
					"entity_id": "light.kitchen",
					"new_state": map[string]interface{}{"state": "on"},
				},
			},
		})

		unsubReq := map[string]interface{}{}
		if err := conn.ReadJSON(&unsubReq); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		if unsubReq["type"] != "unsubscribe_events" {
			reportHandlerErr(errCh, errors.New("expected unsubscribe_events"))
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{
			"id":      unsubReq["id"],
			"type":    "result",
			"success": true,
			"result":  true,
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := ws.WaitForState(ctx, "light.kitchen", func(s State) bool {
		return s.State == "on"
	})
	if err != nil {
		t.Fatalf("wait for state: %v", err)
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSWaitForStateEquals(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()

		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

		subReq := map[string]interface{}{}
		if err := conn.ReadJSON(&subReq); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{
			"id":      subReq["id"],
			"type":    "result",
			"success": true,
			"result":  nil,
		})

		_ = conn.WriteJSON(map[string]interface{}{
			"id":   subReq["id"],
			"type": "event",
			"event": map[string]interface{}{
				"event_type": "state_changed",
				"data": map[string]interface{}{
					"entity_id": "light.kitchen",
					"new_state": map[string]interface{}{"state": "off"},
				},
			},
		})
		_ = conn.WriteJSON(map[string]interface{}{
			"id":   subReq["id"],
			"type": "event",
			"event": map[string]interface{}{
				"event_type": "state_changed",
				"data": map[string]interface{}{
					"entity_id": "light.kitchen",
					"new_state": map[string]interface{}{"state": "on"},
				},
			},
		})

		unsubReq := map[string]interface{}{}
		if err := conn.ReadJSON(&unsubReq); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		if unsubReq["type"] != "unsubscribe_events" {
			reportHandlerErr(errCh, errors.New("expected unsubscribe_events"))
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{
			"id":      unsubReq["id"],
			"type":    "result",
			"success": true,
			"result":  true,
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ws.WaitForStateEquals(ctx, "light.kitchen", "on"); err != nil {
		t.Fatalf("wait for state equals: %v", err)
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSWaitForStateIn(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()

		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

		subReq := map[string]interface{}{}
		if err := conn.ReadJSON(&subReq); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{
			"id":      subReq["id"],
			"type":    "result",
			"success": true,
			"result":  nil,
		})

		_ = conn.WriteJSON(map[string]interface{}{
			"id":   subReq["id"],
			"type": "event",
			"event": map[string]interface{}{
				"event_type": "state_changed",
				"data": map[string]interface{}{
					"entity_id": "light.kitchen",
					"new_state": map[string]interface{}{"state": "off"},
				},
			},
		})
		_ = conn.WriteJSON(map[string]interface{}{
			"id":   subReq["id"],
			"type": "event",
			"event": map[string]interface{}{
				"event_type": "state_changed",
				"data": map[string]interface{}{
					"entity_id": "light.kitchen",
					"new_state": map[string]interface{}{"state": "unavailable"},
				},
			},
		})

		unsubReq := map[string]interface{}{}
		if err := conn.ReadJSON(&unsubReq); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		if unsubReq["type"] != "unsubscribe_events" {
			reportHandlerErr(errCh, errors.New("expected unsubscribe_events"))
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{
			"id":      unsubReq["id"],
			"type":    "result",
			"success": true,
			"result":  true,
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ws.WaitForStateIn(ctx, "light.kitchen", "on", "unavailable"); err != nil {
		t.Fatalf("wait for state in: %v", err)
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSWaitForStateInValidation(t *testing.T) {
	t.Parallel()

	ws := &WSClient{}

	if err := ws.WaitForStateIn(context.Background(), "light.kitchen"); err == nil {
		t.Fatalf("expected validation error for empty states")
	}
	if err := ws.WaitForStateIn(context.Background(), "light.kitchen", "on", ""); err == nil {
		t.Fatalf("expected validation error for empty state value")
	}
	if err := ws.WaitForStateIn(context.Background(), "", "on"); !errors.Is(err, ErrEmptyEntityID) {
		t.Fatalf("expected ErrEmptyEntityID, got: %v", err)
	}
}

func TestWSCallServiceForEntity(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

		req := map[string]interface{}{}
		if err := conn.ReadJSON(&req); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		if req["type"] != "call_service" {
			reportHandlerErr(errCh, errors.New("expected call_service request"))
			return
		}
		serviceData, ok := req["service_data"].(map[string]interface{})
		if !ok {
			reportHandlerErr(errCh, errors.New("missing service_data"))
			return
		}
		if serviceData["entity_id"] != "light.kitchen" {
			reportHandlerErr(errCh, errors.New("unexpected entity_id"))
			return
		}
		if serviceData["brightness"] != float64(200) {
			reportHandlerErr(errCh, errors.New("unexpected brightness"))
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{
			"id":      req["id"],
			"type":    "result",
			"success": true,
			"result": map[string]interface{}{
				"context": map[string]interface{}{
					"id": "ctx-1",
				},
			},
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	result, err := ws.CallServiceForEntity(context.Background(), "light", "turn_on", "light.kitchen", map[string]interface{}{
		"brightness": 200,
	})
	if err != nil {
		t.Fatalf("call service: %v", err)
	}
	if result.Context.ID != "ctx-1" {
		t.Fatalf("unexpected context id: %s", result.Context.ID)
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSEventStateChangedMissingNewState(t *testing.T) {
	t.Parallel()

	ev := WSEvent{
		EventType: EventTypeStateChanged,
		Data:      json.RawMessage(`{"entity_id":"light.kitchen"}`),
	}
	_, ok, err := ev.StateChanged()
	if err != nil || ok {
		t.Fatalf("expected ok=false err=nil for missing new_state: ok=%t err=%v", ok, err)
	}
}

func TestDecodeEventData(t *testing.T) {
	t.Parallel()

	type payload struct {
		EntityID string `json:"entity_id"`
	}
	ev := WSEvent{
		Data: json.RawMessage(`{"entity_id":"light.kitchen"}`),
	}
	got, err := DecodeEventData[payload](ev)
	if err != nil {
		t.Fatalf("decode event: %v", err)
	}
	if got.EntityID != "light.kitchen" {
		t.Fatalf("unexpected entity id: %s", got.EntityID)
	}
}

func TestWSEventStateChanged(t *testing.T) {
	t.Parallel()

	ev := WSEvent{
		EventType: EventTypeStateChanged,
		Data: json.RawMessage(`{
			"entity_id":"light.kitchen",
			"old_state":{"state":"off"},
			"new_state":{"state":"on"}
		}`),
	}
	got, ok, err := ev.StateChanged()
	if err != nil {
		t.Fatalf("state changed: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if got.EntityID != "light.kitchen" {
		t.Fatalf("unexpected entity id: %s", got.EntityID)
	}
	if got.OldState == nil || got.NewState == nil {
		t.Fatalf("expected old/new state")
	}
	if got.OldState.State != "off" || got.NewState.State != "on" {
		t.Fatalf("unexpected states: old=%s new=%s", got.OldState.State, got.NewState.State)
	}
}

func TestWSEventCallServiceEvent(t *testing.T) {
	t.Parallel()

	ev := WSEvent{
		EventType: EventTypeCallService,
		Data: json.RawMessage(`{
			"domain":"light",
			"service":"turn_on",
			"service_data":{"entity_id":"light.kitchen","brightness":180}
		}`),
	}

	got, ok, err := ev.CallServiceEvent()
	if err != nil {
		t.Fatalf("call service event: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if got.Domain != "light" || got.Service != "turn_on" {
		t.Fatalf("unexpected call_service payload: %#v", got)
	}
	if got.ServiceData["entity_id"] != "light.kitchen" {
		t.Fatalf("unexpected service_data: %#v", got.ServiceData)
	}
}

func TestWSEventCallServiceEventNonCallServiceEvent(t *testing.T) {
	t.Parallel()

	ev := WSEvent{
		EventType: EventTypeStateChanged,
		Data:      json.RawMessage(`{"domain":"light","service":"turn_on"}`),
	}
	if _, ok, err := ev.CallServiceEvent(); err != nil || ok {
		t.Fatalf("expected ok=false err=nil, got ok=%t err=%v", ok, err)
	}
}

func TestWSEventStateChangedNonStateEvent(t *testing.T) {
	t.Parallel()

	ev := WSEvent{
		EventType: EventTypeHomeAssistantStart,
		Data:      json.RawMessage(`{"foo":"bar"}`),
	}
	_, ok, err := ev.StateChanged()
	if err != nil || ok {
		t.Fatalf("expected ok=false err=nil, got ok=%t err=%v", ok, err)
	}
}

func TestWSSubscribeCancelUnsubscribes(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	ready := make(chan struct{})
	sendResult := make(chan struct{})
	unsubscribed := make(chan struct{})
	const subscriptionID int64 = 99
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()

		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

		subReq := map[string]interface{}{}
		if err := conn.ReadJSON(&subReq); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		close(ready)

		<-sendResult
		_ = conn.WriteJSON(map[string]interface{}{
			"id":      subReq["id"],
			"type":    "result",
			"success": true,
			"result":  subscriptionID,
		})

		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		unsubReq := map[string]interface{}{}
		if err := conn.ReadJSON(&unsubReq); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		if unsubReq["type"] != "unsubscribe_events" {
			reportHandlerErr(errCh, errors.New("expected unsubscribe_events"))
			return
		}
		if unsubReq["subscription"] != float64(subscriptionID) {
			reportHandlerErr(errCh, errors.New("unexpected subscription id"))
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{
			"id":      unsubReq["id"],
			"type":    "result",
			"success": true,
			"result":  true,
		})
		close(unsubscribed)
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	resultErr := make(chan error, 1)
	go func() {
		_, err := ws.SubscribeEvents(ctx, "state_changed")
		resultErr <- err
	}()

	<-ready
	cancel()
	close(sendResult)

	select {
	case err := <-resultErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for subscribe cancel")
	}

	select {
	case <-unsubscribed:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for unsubscribe")
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSAuthInvalid(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{
			"type":    "auth_invalid",
			"message": "invalid token",
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "bad-token")
	err := ws.Connect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid token") {
		t.Fatalf("expected auth error, got: %v", err)
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSGetStates(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

		req := map[string]interface{}{}
		if err := conn.ReadJSON(&req); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		if req["type"] != "get_states" {
			reportHandlerErr(errCh, errors.New("expected get_states request"))
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{
			"id":      req["id"],
			"type":    "result",
			"success": true,
			"result": []map[string]interface{}{
				{"entity_id": "light.kitchen", "state": "on"},
			},
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	states, err := ws.GetStates(context.Background())
	if err != nil {
		t.Fatalf("get states: %v", err)
	}
	if len(states) != 1 || states[0].EntityID != "light.kitchen" {
		t.Fatalf("unexpected states: %#v", states)
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSCallServiceWithResponse(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

		req := map[string]interface{}{}
		if err := conn.ReadJSON(&req); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		if req["type"] != "call_service" {
			reportHandlerErr(errCh, errors.New("expected call_service request"))
			return
		}
		if req["domain"] != "weather" || req["service"] != "get_forecasts" {
			reportHandlerErr(errCh, errors.New("unexpected call_service target"))
			return
		}
		if req["return_response"] != true {
			reportHandlerErr(errCh, errors.New("missing return_response flag"))
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{
			"id":      req["id"],
			"type":    "result",
			"success": true,
			"result": map[string]interface{}{
				"context": map[string]interface{}{
					"id": "ctx-1",
				},
				"service_response": map[string]interface{}{
					"weather.home": map[string]interface{}{
						"forecast": []map[string]interface{}{
							{"condition": "sunny"},
						},
					},
				},
			},
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	result, err := ws.CallServiceWithResponse(context.Background(), "weather", "get_forecasts", map[string]interface{}{
		"entity_id": "weather.home",
		"type":      "daily",
	})
	if err != nil {
		t.Fatalf("call service with response: %v", err)
	}
	raw, ok := result.ServiceResponse["weather.home"]
	if !ok {
		t.Fatalf("missing service response: %#v", result.ServiceResponse)
	}
	if !strings.Contains(string(raw), `"condition":"sunny"`) {
		t.Fatalf("unexpected service response: %s", string(raw))
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSFireEventEmptyEventType(t *testing.T) {
	t.Parallel()

	ws := NewWSClient(ClientConfig{Host: "http://localhost:8123", Token: "token"})
	err := ws.FireEvent(context.Background(), "", nil)
	if !errors.Is(err, ErrEmptyEventType) {
		t.Fatalf("expected ErrEmptyEventType, got: %v", err)
	}
}

func TestWSDoReturnsWSError(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

		req := map[string]interface{}{}
		if err := conn.ReadJSON(&req); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{
			"id":      req["id"],
			"type":    "result",
			"success": false,
			"error": map[string]interface{}{
				"code":    "not_found",
				"message": "service not found",
			},
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	_, err := ws.CallService(context.Background(), "light", "does_not_exist", nil)
	if err == nil || !strings.Contains(err.Error(), "service not found") {
		t.Fatalf("expected ws error, got: %v", err)
	}
	assertNoHandlerErr(t, errCh)
}

func newWSTestServer(t *testing.T, errCh chan error, fn func(conn *websocket.Conn)) *httptest.Server {
	t.Helper()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		fn(conn)
	}))
	return server
}

// handshakeAndServe performs the auth_required/auth/auth_ok handshake and then
// delegates to the per-test handler. It is a small helper to keep new command
// tests concise.
func handshakeAndServe(errCh chan error, conn *websocket.Conn, handle func(req map[string]interface{}) (response map[string]interface{}, ok bool)) {
	defer conn.Close()
	_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
	_ = conn.ReadJSON(&map[string]interface{}{})
	_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

	req := map[string]interface{}{}
	if err := conn.ReadJSON(&req); err != nil {
		reportHandlerErr(errCh, err)
		return
	}
	resp, ok := handle(req)
	if !ok {
		return
	}
	if resp == nil {
		resp = map[string]interface{}{"type": "result", "success": true}
	}
	if _, has := resp["id"]; !has {
		resp["id"] = req["id"]
	}
	_ = conn.WriteJSON(resp)
}

func TestWSClient_DeclareSupportedFeatures(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		handshakeAndServe(errCh, conn, func(req map[string]interface{}) (map[string]interface{}, bool) {
			if req["type"] != "supported_features" {
				reportHandlerErr(errCh, errors.New("expected supported_features"))
				return nil, false
			}
			features, ok := req["features"].(map[string]interface{})
			if !ok || features["coalesce_messages"].(float64) != 1 {
				reportHandlerErr(errCh, errors.New("expected coalesce_messages=1"))
				return nil, false
			}
			return nil, true
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	if err := ws.DeclareSupportedFeatures(context.Background(), map[string]interface{}{
		"coalesce_messages": 1,
	}); err != nil {
		t.Fatalf("declare supported features: %v", err)
	}
	assertNoHandlerErr(t, errCh)
}

// TestWSClient_Connect_NoAutoSupportedFeatures verifies Connect does not
// automatically send supported_features (backwards compatibility).
func TestWSClient_Connect_NoAutoSupportedFeatures(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

		// Expect the next message to be ping, not supported_features.
		msg := map[string]interface{}{}
		if err := conn.ReadJSON(&msg); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		if msg["type"] == "supported_features" {
			reportHandlerErr(errCh, errors.New("Connect must not send supported_features automatically"))
			return
		}
		if msg["type"] != "ping" {
			reportHandlerErr(errCh, errors.New("expected ping as first post-auth message"))
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{"type": "pong", "id": msg["id"]})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	if err := ws.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSClient_GetPanels(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		handshakeAndServe(errCh, conn, func(req map[string]interface{}) (map[string]interface{}, bool) {
			if req["type"] != "get_panels" {
				reportHandlerErr(errCh, errors.New("expected get_panels"))
				return nil, false
			}
			return map[string]interface{}{
				"type":    "result",
				"success": true,
				"result": map[string]interface{}{
					"lovelace": map[string]interface{}{
						"component_name": "lovelace",
						"url_path":       "lovelace",
						"require_admin":  false,
					},
					"config": map[string]interface{}{
						"component_name": "config",
						"url_path":       "config",
						"icon":           "mdi:cog",
						"require_admin":  true,
					},
				},
			}, true
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	panels, err := ws.GetPanels(context.Background())
	if err != nil {
		t.Fatalf("get panels: %v", err)
	}
	if len(panels) != 2 {
		t.Fatalf("expected 2 panels, got %d", len(panels))
	}
	if panels["config"].Icon != "mdi:cog" || !panels["config"].RequireAdmin {
		t.Fatalf("unexpected config panel: %#v", panels["config"])
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSClient_GetPanels_Error(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		handshakeAndServe(errCh, conn, func(req map[string]interface{}) (map[string]interface{}, bool) {
			return map[string]interface{}{
				"type":    "result",
				"success": false,
				"error":   map[string]interface{}{"code": "unknown_error", "message": "boom"},
			}, true
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	_, err := ws.GetPanels(context.Background())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected ws error, got: %v", err)
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSClient_ValidateConfig_Valid(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		handshakeAndServe(errCh, conn, func(req map[string]interface{}) (map[string]interface{}, bool) {
			if req["type"] != "validate_config" {
				reportHandlerErr(errCh, errors.New("expected validate_config"))
				return nil, false
			}
			if _, has := req["trigger"]; !has {
				reportHandlerErr(errCh, errors.New("missing trigger in request"))
				return nil, false
			}
			return map[string]interface{}{
				"type":    "result",
				"success": true,
				"result": map[string]interface{}{
					"trigger": map[string]interface{}{"valid": true},
				},
			}, true
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	res, err := ws.ValidateConfig(context.Background(), ValidateConfigRequest{
		Trigger: map[string]interface{}{"platform": "state", "entity_id": "light.kitchen"},
	})
	if err != nil {
		t.Fatalf("validate config: %v", err)
	}
	if res.Trigger == nil || !res.Trigger.Valid {
		t.Fatalf("expected valid trigger, got %#v", res.Trigger)
	}
	if res.Condition != nil || res.Action != nil {
		t.Fatalf("expected nil condition/action, got %#v", res)
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSClient_ValidateConfig_Invalid(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		handshakeAndServe(errCh, conn, func(req map[string]interface{}) (map[string]interface{}, bool) {
			return map[string]interface{}{
				"type":    "result",
				"success": true,
				"result": map[string]interface{}{
					"action": map[string]interface{}{"valid": false, "error": "unknown service foo.bar"},
				},
			}, true
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	res, err := ws.ValidateConfig(context.Background(), ValidateConfigRequest{
		Action: map[string]interface{}{"service": "foo.bar"},
	})
	if err != nil {
		t.Fatalf("validate config: %v", err)
	}
	if res.Action == nil || res.Action.Valid || res.Action.Error == "" {
		t.Fatalf("expected invalid action, got %#v", res.Action)
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSClient_ExtractFromTarget(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		handshakeAndServe(errCh, conn, func(req map[string]interface{}) (map[string]interface{}, bool) {
			if req["type"] != "extract_from_target" {
				reportHandlerErr(errCh, errors.New("expected extract_from_target"))
				return nil, false
			}
			if req["expand_group"] != false {
				reportHandlerErr(errCh, errors.New("expected expand_group=false"))
				return nil, false
			}
			target, _ := req["target"].(map[string]interface{})
			areas, _ := target["area_id"].([]interface{})
			if len(areas) != 1 || areas[0] != "kitchen" {
				reportHandlerErr(errCh, errors.New("unexpected target payload"))
				return nil, false
			}
			return map[string]interface{}{
				"type":    "result",
				"success": true,
				"result": map[string]interface{}{
					"referenced_entities": []string{"light.kitchen", "switch.kitchen"},
					"referenced_areas":    []string{"kitchen"},
					"missing_floors":      []string{},
				},
			}, true
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	res, err := ws.ExtractFromTarget(context.Background(), TargetSelector{AreaID: []string{"kitchen"}}, false)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(res.ReferencedEntities) != 2 || res.ReferencedEntities[0] != "light.kitchen" {
		t.Fatalf("unexpected entities: %#v", res.ReferencedEntities)
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSClient_ExtractFromTarget_EmptyTarget(t *testing.T) {
	t.Parallel()

	ws := NewWSClient(ClientConfig{Host: "http://localhost:8123", Token: "token"})
	if _, err := ws.ExtractFromTarget(context.Background(), TargetSelector{}, false); !errors.Is(err, ErrEmptyTarget) {
		t.Fatalf("expected ErrEmptyTarget, got: %v", err)
	}
}

func TestWSClient_TargetHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		expectType  string
		call        func(ws *WSClient) ([]string, error)
		expandGroup bool
	}{
		{
			name:        "triggers",
			expectType:  "get_triggers_for_target",
			expandGroup: true,
			call: func(ws *WSClient) ([]string, error) {
				return ws.GetTriggersForTarget(context.Background(), TargetSelector{EntityID: []string{"light.kitchen"}}, true)
			},
		},
		{
			name:        "conditions",
			expectType:  "get_conditions_for_target",
			expandGroup: true,
			call: func(ws *WSClient) ([]string, error) {
				return ws.GetConditionsForTarget(context.Background(), TargetSelector{EntityID: []string{"light.kitchen"}}, true)
			},
		},
		{
			name:        "services",
			expectType:  "get_services_for_target",
			expandGroup: false,
			call: func(ws *WSClient) ([]string, error) {
				return ws.GetServicesForTarget(context.Background(), TargetSelector{EntityID: []string{"light.kitchen"}}, false)
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			errCh := make(chan error, 1)
			srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
				handshakeAndServe(errCh, conn, func(req map[string]interface{}) (map[string]interface{}, bool) {
					if req["type"] != tc.expectType {
						reportHandlerErr(errCh, errors.New("unexpected type: "+req["type"].(string)))
						return nil, false
					}
					if req["expand_group"] != tc.expandGroup {
						reportHandlerErr(errCh, errors.New("unexpected expand_group"))
						return nil, false
					}
					return map[string]interface{}{
						"type":    "result",
						"success": true,
						"result":  []string{"light.turned_on", "light.turned_off"},
					}, true
				})
			})
			defer srv.Close()

			ws := newTestWSClient(t, srv.URL, "test-token")
			if err := ws.Connect(context.Background()); err != nil {
				t.Fatalf("connect: %v", err)
			}
			t.Cleanup(func() { _ = ws.Close() })

			res, err := tc.call(ws)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if len(res) != 2 || res[0] != "light.turned_on" {
				t.Fatalf("%s: unexpected result %#v", tc.name, res)
			}
			assertNoHandlerErr(t, errCh)
		})
	}
}

func TestWSClient_TargetHelpers_EmptyTarget(t *testing.T) {
	t.Parallel()

	ws := NewWSClient(ClientConfig{Host: "http://localhost:8123", Token: "token"})
	if _, err := ws.GetTriggersForTarget(context.Background(), TargetSelector{}, true); !errors.Is(err, ErrEmptyTarget) {
		t.Fatalf("triggers: expected ErrEmptyTarget, got: %v", err)
	}
	if _, err := ws.GetConditionsForTarget(context.Background(), TargetSelector{}, true); !errors.Is(err, ErrEmptyTarget) {
		t.Fatalf("conditions: expected ErrEmptyTarget, got: %v", err)
	}
	if _, err := ws.GetServicesForTarget(context.Background(), TargetSelector{}, true); !errors.Is(err, ErrEmptyTarget) {
		t.Fatalf("services: expected ErrEmptyTarget, got: %v", err)
	}
}

func TestWSClient_ListEntityRegistryForDisplay(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		handshakeAndServe(errCh, conn, func(req map[string]interface{}) (map[string]interface{}, bool) {
			if req["type"] != "config/entity_registry/list_for_display" {
				reportHandlerErr(errCh, errors.New("expected config/entity_registry/list_for_display, got: "+req["type"].(string)))
				return nil, false
			}
			return map[string]interface{}{
				"type":    "result",
				"success": true,
				"result": map[string]interface{}{
					"entity_categories": map[string]interface{}{
						"0": "config",
						"1": "diagnostic",
					},
					"entities": []map[string]interface{}{
						{"ei": "light.kitchen", "dn": "Kitchen"},
						{"ei": "sensor.battery", "dn": "Battery", "ec": float64(1)},
					},
				},
			}, true
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	res, err := ws.ListEntityRegistryForDisplay(context.Background())
	if err != nil {
		t.Fatalf("list for display: %v", err)
	}
	if len(res.Entities) != 2 || res.Entities[0]["ei"] != "light.kitchen" {
		t.Fatalf("unexpected entities: %#v", res.Entities)
	}
	if res.EntityCategories["1"] != "diagnostic" {
		t.Fatalf("unexpected entity_categories: %#v", res.EntityCategories)
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSClient_ListExposedEntities(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		handshakeAndServe(errCh, conn, func(req map[string]interface{}) (map[string]interface{}, bool) {
			if req["type"] != "homeassistant/expose_entity/list" {
				reportHandlerErr(errCh, errors.New("expected expose_entity/list"))
				return nil, false
			}
			return map[string]interface{}{
				"type":    "result",
				"success": true,
				"result": map[string]interface{}{
					"exposed_entities": map[string]interface{}{
						"light.kitchen": map[string]interface{}{
							"conversation": true,
							"cloud.alexa":  false,
						},
					},
				},
			}, true
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	res, err := ws.ListExposedEntities(context.Background())
	if err != nil {
		t.Fatalf("list exposed: %v", err)
	}
	exposure := res.ExposedEntities["light.kitchen"]
	if !exposure["conversation"] || exposure["cloud.alexa"] {
		t.Fatalf("unexpected exposure: %#v", exposure)
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSClient_ExposeEntity(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		handshakeAndServe(errCh, conn, func(req map[string]interface{}) (map[string]interface{}, bool) {
			if req["type"] != "homeassistant/expose_entity" {
				reportHandlerErr(errCh, errors.New("expected expose_entity"))
				return nil, false
			}
			ids, _ := req["entity_ids"].([]interface{})
			assistants, _ := req["assistants"].([]interface{})
			if len(ids) != 1 || ids[0] != "light.kitchen" {
				reportHandlerErr(errCh, errors.New("unexpected entity_ids"))
				return nil, false
			}
			if len(assistants) != 1 || assistants[0] != "conversation" {
				reportHandlerErr(errCh, errors.New("unexpected assistants"))
				return nil, false
			}
			if req["should_expose"] != true {
				reportHandlerErr(errCh, errors.New("expected should_expose=true"))
				return nil, false
			}
			return nil, true
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	err := ws.ExposeEntity(context.Background(), ExposeEntityRequest{
		Assistants:   []string{"conversation"},
		EntityIDs:    []string{"light.kitchen"},
		ShouldExpose: true,
	})
	if err != nil {
		t.Fatalf("expose entity: %v", err)
	}
	assertNoHandlerErr(t, errCh)
}

func TestWSClient_ExposeEntity_EmptyEntityIDs(t *testing.T) {
	t.Parallel()

	ws := NewWSClient(ClientConfig{Host: "http://localhost:8123", Token: "token"})
	err := ws.ExposeEntity(context.Background(), ExposeEntityRequest{
		Assistants: []string{"conversation"},
	})
	if !errors.Is(err, ErrEmptyEntityID) {
		t.Fatalf("expected ErrEmptyEntityID, got: %v", err)
	}
}

func TestWSClient_ExposeEntity_EmptyAssistants(t *testing.T) {
	t.Parallel()

	ws := NewWSClient(ClientConfig{Host: "http://localhost:8123", Token: "token"})
	err := ws.ExposeEntity(context.Background(), ExposeEntityRequest{
		EntityIDs: []string{"light.kitchen"},
	})
	if !errors.Is(err, ErrEmptyAssistants) {
		t.Fatalf("expected ErrEmptyAssistants, got: %v", err)
	}
}

// TestWSClient_Do_StillWorks_ForNewCommands verifies raw Do() continues to work
// for the same commands that now have typed wrappers (backwards compatibility).
func TestWSClient_Do_StillWorks_ForNewCommands(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		handshakeAndServe(errCh, conn, func(req map[string]interface{}) (map[string]interface{}, bool) {
			if req["type"] != "get_panels" {
				reportHandlerErr(errCh, errors.New("expected get_panels"))
				return nil, false
			}
			return map[string]interface{}{
				"type":    "result",
				"success": true,
				"result":  map[string]interface{}{"lovelace": map[string]interface{}{"component_name": "lovelace", "url_path": "lovelace"}},
			}, true
		})
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	raw := map[string]json.RawMessage{}
	if err := ws.Do(context.Background(), map[string]interface{}{"type": "get_panels"}, &raw); err != nil {
		t.Fatalf("raw do: %v", err)
	}
	if _, ok := raw["lovelace"]; !ok {
		t.Fatalf("expected lovelace key in raw response: %#v", raw)
	}
	assertNoHandlerErr(t, errCh)
}

func TestDecodeIncomingFrame_Strict(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		input  string
		count  int
		wantOK bool
	}{
		{"object", `{"type":"result","id":1,"success":true}`, 1, true},
		{"array", `[{"type":"event","id":1},{"type":"pong","id":2}]`, 2, true},
		{"leading whitespace then object", "  \n\t{\"type\":\"pong\"}", 1, true},
		{"null literal rejected", `null`, 0, false},
		{"number literal rejected", `42`, 0, false},
		{"string literal rejected", `"oops"`, 0, false},
		{"empty frame rejected", ``, 0, false},
		{"whitespace-only rejected", "   \n", 0, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			msgs, err := decodeIncomingFrame([]byte(tc.input))
			if tc.wantOK {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				if len(msgs) != tc.count {
					t.Fatalf("count: got %d want %d", len(msgs), tc.count)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error, got msgs=%v", msgs)
			}
		})
	}
}

// TestWSClient_NonTextFrame_FailsPending verifies that a binary frame is
// treated as a protocol violation (HA only sends text frames).
func TestWSClient_NonTextFrame_FailsPending(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()

		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

		req := map[string]interface{}{}
		if err := conn.ReadJSON(&req); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte("\x00\x01\x02"))
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := ws.Do(ctx, map[string]interface{}{"type": "get_panels"}, nil)
	if err == nil {
		t.Fatalf("expected error from pending Do, got nil")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Do should not block on binary frame: %v", err)
	}
	assertNoHandlerErr(t, errCh)
}

// TestWSClient_MalformedFrame_FailsPending verifies that a non-JSON frame is
// treated as a fatal protocol error: pending callers get the decode error
// rather than blocking until their context expires.
func TestWSClient_MalformedFrame_FailsPending(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()

		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

		req := map[string]interface{}{}
		if err := conn.ReadJSON(&req); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		// Reply with garbage that is neither a JSON object nor array.
		_ = conn.WriteMessage(websocket.TextMessage, []byte("not-json"))
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := ws.Do(ctx, map[string]interface{}{"type": "get_panels"}, nil)
	if err == nil {
		t.Fatalf("expected error from pending Do, got nil")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Do should not block until ctx deadline on malformed frame: %v", err)
	}
	assertNoHandlerErr(t, errCh)
}

// TestWSClient_CoalescedMessages exercises readLoop on a single text frame
// that contains a JSON array of messages, which is what HA sends after
// coalesce_messages is enabled via supported_features.
func TestWSClient_CoalescedMessages(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	srv := newWSTestServer(t, errCh, func(conn *websocket.Conn) {
		defer conn.Close()

		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_required"})
		_ = conn.ReadJSON(&map[string]interface{}{})
		_ = conn.WriteJSON(map[string]interface{}{"type": "auth_ok"})

		subReq := map[string]interface{}{}
		if err := conn.ReadJSON(&subReq); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		if subReq["type"] != "subscribe_events" {
			reportHandlerErr(errCh, errors.New("expected subscribe_events"))
			return
		}

		batch := []map[string]interface{}{
			{"id": subReq["id"], "type": "result", "success": true, "result": nil},
			{"id": subReq["id"], "type": "event", "event": map[string]interface{}{
				"event_type": "state_changed",
				"data":       map[string]interface{}{"entity_id": "light.kitchen"},
			}},
			{"id": subReq["id"], "type": "event", "event": map[string]interface{}{
				"event_type": "state_changed",
				"data":       map[string]interface{}{"entity_id": "light.living_room"},
			}},
		}
		payload, err := json.Marshal(batch)
		if err != nil {
			reportHandlerErr(errCh, err)
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			reportHandlerErr(errCh, err)
			return
		}
	})
	defer srv.Close()

	ws := newTestWSClient(t, srv.URL, "test-token")
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	sub, err := ws.SubscribeEvents(context.Background(), "state_changed")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	seen := make([]string, 0, 2)
	for len(seen) < 2 {
		select {
		case ev := <-sub.Events():
			if ev.EventType != "state_changed" {
				t.Fatalf("unexpected event type: %s", ev.EventType)
			}
			seen = append(seen, string(ev.Data))
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout: only got %d coalesced events", len(seen))
		}
	}
	if !strings.Contains(seen[0], "light.kitchen") || !strings.Contains(seen[1], "light.living_room") {
		t.Fatalf("unexpected coalesced events: %v", seen)
	}
	assertNoHandlerErr(t, errCh)
}
