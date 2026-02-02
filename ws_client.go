package go_ha_client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var (
	ErrWSNotConnected   = errors.New("ws client is not connected")
	ErrWSClosed         = errors.New("ws connection closed")
	ErrWSAuthFailed     = errors.New("ws authentication failed")
	ErrWSInvalidRequest = errors.New("ws request must include non-empty type")
)

type WSError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *WSError) Error() string {
	if e == nil {
		return "ws request failed"
	}
	if e.Code == "" {
		return e.Message
	}
	if e.Message == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

type WSEvent struct {
	SubscriptionID int64
	EventType      string
	Data           json.RawMessage
	Raw            json.RawMessage
}

type WSSubscription struct {
	id     int64
	events chan WSEvent
	errors chan error
	client *WSClient
	once   sync.Once
}

type WSCallServiceResult struct {
	Context struct {
		ID       string `json:"id"`
		ParentID string `json:"parent_id"`
		UserID   string `json:"user_id"`
	} `json:"context"`
	ServiceResponse map[string]json.RawMessage `json:"service_response,omitempty"`
}

func (s *WSSubscription) ID() int64 {
	return s.id
}

func (s *WSSubscription) Events() <-chan WSEvent {
	return s.events
}

func (s *WSSubscription) Errors() <-chan error {
	return s.errors
}

func (s *WSSubscription) Unsubscribe(ctx context.Context) error {
	return s.client.unsubscribe(ctx, s.id)
}

type wsIncomingMessage struct {
	Type    string          `json:"type"`
	ID      int64           `json:"id,omitempty"`
	Success bool            `json:"success,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *WSError        `json:"error,omitempty"`
	Event   json.RawMessage `json:"event,omitempty"`
	Message string          `json:"message,omitempty"`
}

type wsEventPayload struct {
	EventType string          `json:"event_type"`
	Data      json.RawMessage `json:"data"`
}

type wsPendingResult struct {
	msg wsIncomingMessage
	err error
}

type WSClient struct {
	config ClientConfig
	dialer *websocket.Dialer

	mu      sync.RWMutex
	conn    *websocket.Conn
	pending map[int64]chan wsPendingResult
	subs    map[int64]*WSSubscription

	writeMu sync.Mutex
	nextID  int64
}

func NewWSClient(config ClientConfig) *WSClient {
	return &WSClient{
		config: config,
		dialer: websocket.DefaultDialer,
	}
}

func (c *Client) WS() *WSClient {
	return NewWSClient(c.config)
}

func (c *WSClient) Connect(ctx context.Context) error {
	c.mu.RLock()
	if c.conn != nil {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	wsURL, err := websocketURL(c.config.Host)
	if err != nil {
		return err
	}

	conn, _, err := c.dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return err
	}

	msg := wsIncomingMessage{}
	if err := conn.ReadJSON(&msg); err != nil {
		_ = conn.Close()
		return err
	}
	if msg.Type != "auth_required" {
		_ = conn.Close()
		return fmt.Errorf("unexpected ws handshake message: %s", msg.Type)
	}

	if err := conn.WriteJSON(map[string]interface{}{
		"type":         "auth",
		"access_token": c.config.Token,
	}); err != nil {
		_ = conn.Close()
		return err
	}

	msg = wsIncomingMessage{}
	if err := conn.ReadJSON(&msg); err != nil {
		_ = conn.Close()
		return err
	}
	if msg.Type != "auth_ok" {
		_ = conn.Close()
		if msg.Type == "auth_invalid" {
			return fmt.Errorf("%w: %s", ErrWSAuthFailed, msg.Message)
		}
		return fmt.Errorf("unexpected ws auth message: %s", msg.Type)
	}

	c.mu.Lock()
	c.conn = conn
	c.pending = make(map[int64]chan wsPendingResult)
	c.subs = make(map[int64]*WSSubscription)
	c.nextID = 0
	c.mu.Unlock()

	go c.readLoop(conn)
	return nil
}

func (c *WSClient) Close() error {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()

	if conn != nil {
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
		_ = conn.Close()
	}
	c.failAll(ErrWSClosed)
	return nil
}

func (c *WSClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn != nil
}

func (c *WSClient) Ping(ctx context.Context) error {
	return c.Do(ctx, map[string]interface{}{
		"type": "ping",
	}, nil)
}

func (c *WSClient) GetStates(ctx context.Context) (StateEntities, error) {
	states := StateEntities{}
	return states, c.Do(ctx, map[string]interface{}{
		"type": "get_states",
	}, &states)
}

func (c *WSClient) GetConfig(ctx context.Context) (Config, error) {
	cfg := Config{}
	return cfg, c.Do(ctx, map[string]interface{}{
		"type": "get_config",
	}, &cfg)
}

func (c *WSClient) GetServices(ctx context.Context) (Services, error) {
	services := Services{}
	return services, c.Do(ctx, map[string]interface{}{
		"type": "get_services",
	}, &services)
}

func (c *WSClient) FireEvent(ctx context.Context, eventType string, data map[string]interface{}) error {
	if eventType == "" {
		return ErrEmptyEventType
	}
	req := map[string]interface{}{
		"type":       "fire_event",
		"event_type": eventType,
	}
	if data != nil {
		req["event_data"] = data
	}
	return c.Do(ctx, req, nil)
}

func (c *WSClient) CallService(ctx context.Context, domain, service string, data map[string]interface{}) (WSCallServiceResult, error) {
	result := WSCallServiceResult{}
	if service == "" {
		return result, ErrEmptyService
	}
	if domain == "" {
		return result, ErrEmptyDomain
	}
	req := map[string]interface{}{
		"type":    "call_service",
		"domain":  domain,
		"service": service,
	}
	if data != nil {
		req["service_data"] = data
	}
	return result, c.Do(ctx, req, &result)
}

func (c *WSClient) CallServiceWithResponse(ctx context.Context, domain, service string, data map[string]interface{}) (WSCallServiceResult, error) {
	result := WSCallServiceResult{}
	if service == "" {
		return result, ErrEmptyService
	}
	if domain == "" {
		return result, ErrEmptyDomain
	}
	req := map[string]interface{}{
		"type":            "call_service",
		"domain":          domain,
		"service":         service,
		"return_response": true,
	}
	if data != nil {
		req["service_data"] = data
	}
	return result, c.Do(ctx, req, &result)
}

func (c *WSClient) Do(ctx context.Context, req map[string]interface{}, out interface{}) error {
	resp, _, err := c.send(ctx, req)
	if err != nil {
		return err
	}

	switch resp.Type {
	case "pong":
		return nil
	case "result":
		if !resp.Success {
			if resp.Error != nil {
				return resp.Error
			}
			return errors.New("ws request failed")
		}
		if out == nil || len(resp.Result) == 0 || string(resp.Result) == "null" {
			return nil
		}
		if err := json.Unmarshal(resp.Result, out); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unexpected ws response type: %s", resp.Type)
	}
}

func (c *WSClient) SubscribeEvents(ctx context.Context, eventType string) (*WSSubscription, error) {
	req := map[string]interface{}{
		"type": "subscribe_events",
	}
	if eventType != "" {
		req["event_type"] = eventType
	}
	return c.subscribe(ctx, req)
}

func (c *WSClient) SubscribeTrigger(ctx context.Context, trigger interface{}) (*WSSubscription, error) {
	if trigger == nil {
		return nil, errors.New("trigger must not be nil")
	}
	return c.subscribe(ctx, map[string]interface{}{
		"type":    "subscribe_trigger",
		"trigger": trigger,
	})
}

func (c *WSClient) subscribe(ctx context.Context, req map[string]interface{}) (*WSSubscription, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	if req == nil || req["type"] == "" {
		return nil, ErrWSInvalidRequest
	}

	sub := &WSSubscription{
		id:     id,
		events: make(chan WSEvent, 32),
		errors: make(chan error, 1),
		client: c,
	}

	respCh := make(chan wsPendingResult, 1)
	payload := cloneWSRequest(req)
	payload["id"] = id

	c.mu.Lock()
	if c.conn == nil {
		c.mu.Unlock()
		return nil, ErrWSNotConnected
	}
	c.pending[id] = respCh
	c.subs[id] = sub
	c.mu.Unlock()

	if err := c.writeJSON(payload); err != nil {
		c.cleanupPending(id)
		c.cleanupSubscription(id, false, nil)
		return nil, err
	}

	select {
	case <-ctx.Done():
		c.cleanupPending(id)
		c.cleanupSubscription(id, false, nil)
		return nil, ctx.Err()
	case res := <-respCh:
		if res.err != nil {
			c.cleanupSubscription(id, false, nil)
			return nil, res.err
		}
		if res.msg.Type != "result" || !res.msg.Success {
			c.cleanupSubscription(id, false, nil)
			if res.msg.Error != nil {
				return nil, res.msg.Error
			}
			return nil, errors.New("ws subscribe failed")
		}
		return sub, nil
	}
}

func (c *WSClient) unsubscribe(ctx context.Context, id int64) error {
	err := c.Do(ctx, map[string]interface{}{
		"type":         "unsubscribe_events",
		"subscription": id,
	}, nil)
	if err != nil {
		return err
	}
	c.cleanupSubscription(id, true, nil)
	return nil
}

func (c *WSClient) send(ctx context.Context, req map[string]interface{}) (wsIncomingMessage, int64, error) {
	if req == nil || req["type"] == "" {
		return wsIncomingMessage{}, 0, ErrWSInvalidRequest
	}

	id := atomic.AddInt64(&c.nextID, 1)
	respCh := make(chan wsPendingResult, 1)
	payload := cloneWSRequest(req)
	payload["id"] = id

	c.mu.Lock()
	if c.conn == nil {
		c.mu.Unlock()
		return wsIncomingMessage{}, 0, ErrWSNotConnected
	}
	c.pending[id] = respCh
	c.mu.Unlock()

	if err := c.writeJSON(payload); err != nil {
		c.cleanupPending(id)
		return wsIncomingMessage{}, 0, err
	}

	select {
	case <-ctx.Done():
		c.cleanupPending(id)
		return wsIncomingMessage{}, id, ctx.Err()
	case res := <-respCh:
		if res.err != nil {
			return wsIncomingMessage{}, id, res.err
		}
		return res.msg, id, nil
	}
}

func (c *WSClient) readLoop(conn *websocket.Conn) {
	for {
		msg := wsIncomingMessage{}
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) || errors.Is(err, io.EOF) {
				c.failAll(ErrWSClosed)
				return
			}
			c.failAll(err)
			return
		}

		switch msg.Type {
		case "result", "pong":
			c.dispatchPending(msg)
		case "event":
			c.dispatchEvent(msg)
		}
	}
}

func (c *WSClient) dispatchPending(msg wsIncomingMessage) {
	c.mu.Lock()
	ch, ok := c.pending[msg.ID]
	if ok {
		delete(c.pending, msg.ID)
	}
	c.mu.Unlock()

	if !ok {
		return
	}
	ch <- wsPendingResult{msg: msg}
	close(ch)
}

func (c *WSClient) dispatchEvent(msg wsIncomingMessage) {
	c.mu.RLock()
	sub := c.subs[msg.ID]
	c.mu.RUnlock()
	if sub == nil {
		return
	}

	event := WSEvent{
		SubscriptionID: msg.ID,
		Raw:            msg.Event,
	}
	payload := wsEventPayload{}
	if err := json.Unmarshal(msg.Event, &payload); err == nil {
		event.EventType = payload.EventType
		event.Data = payload.Data
	}

	select {
	case sub.events <- event:
	default:
		select {
		case sub.errors <- errors.New("ws subscription event buffer is full"):
		default:
		}
	}
}

func (c *WSClient) cleanupPending(id int64) {
	c.mu.Lock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if ok {
		close(ch)
	}
}

func (c *WSClient) cleanupSubscription(id int64, closeChannels bool, reportErr error) {
	c.mu.Lock()
	sub, ok := c.subs[id]
	if ok {
		delete(c.subs, id)
	}
	c.mu.Unlock()
	if !ok {
		return
	}
	if reportErr != nil {
		select {
		case sub.errors <- reportErr:
		default:
		}
	}
	if closeChannels {
		sub.once.Do(func() {
			close(sub.events)
			close(sub.errors)
		})
	}
}

func (c *WSClient) failAll(err error) {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	pending := c.pending
	c.pending = make(map[int64]chan wsPendingResult)
	subs := c.subs
	c.subs = make(map[int64]*WSSubscription)
	c.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}

	for _, ch := range pending {
		ch <- wsPendingResult{err: err}
		close(ch)
	}
	for _, sub := range subs {
		if err != nil {
			select {
			case sub.errors <- err:
			default:
			}
		}
		sub.once.Do(func() {
			close(sub.events)
			close(sub.errors)
		})
	}
}

func (c *WSClient) writeJSON(v interface{}) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return ErrWSNotConnected
	}
	return conn.WriteJSON(v)
}

func websocketURL(host string) (string, error) {
	if host == "" {
		return "", errors.New("host must not be empty")
	}
	u, err := url.Parse(host)
	if err != nil {
		return "", err
	}

	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("unsupported scheme for host: %s", u.Scheme)
	}

	base := strings.TrimRight(u.Path, "/")
	u.Path = base + "/api/websocket"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func cloneWSRequest(req map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(req)+1)
	for k, v := range req {
		out[k] = v
	}
	return out
}
