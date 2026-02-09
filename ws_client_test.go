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
