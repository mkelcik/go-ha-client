package go_ha_client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

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

	client := NewClient(ClientConfig{Host: srv.URL, Token: "test-token"}, &http.Client{})
	ws := client.WS()
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	if err := ws.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
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

	client := NewClient(ClientConfig{Host: srv.URL, Token: "test-token"}, &http.Client{})
	ws := client.WS()
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

	client := NewClient(ClientConfig{Host: srv.URL, Token: "bad-token"}, &http.Client{})
	ws := client.WS()
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

	client := NewClient(ClientConfig{Host: srv.URL, Token: "test-token"}, &http.Client{})
	ws := client.WS()
	if err := ws.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	states, err := ws.GetStates(context.Background())
	if err != nil {
		t.Fatalf("get states: %v", err)
	}
	if len(states) != 1 || states[0].EntityId != "light.kitchen" {
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

	client := NewClient(ClientConfig{Host: srv.URL, Token: "test-token"}, &http.Client{})
	ws := client.WS()
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

	client := NewClient(ClientConfig{Host: srv.URL, Token: "test-token"}, &http.Client{})
	ws := client.WS()
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
