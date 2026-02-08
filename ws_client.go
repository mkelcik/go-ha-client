package go_ha_client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var (
	// ErrWSNotConnected indicates the WebSocket client is not connected.
	ErrWSNotConnected = errors.New("ws client is not connected")
	// ErrWSClosed indicates the WebSocket connection was closed.
	ErrWSClosed = errors.New("ws connection closed")
	// ErrWSAuthFailed indicates authentication failed.
	ErrWSAuthFailed = errors.New("ws authentication failed")
	// ErrWSInvalidRequest indicates a request is missing a required type.
	ErrWSInvalidRequest = errors.New("ws request must include non-empty type")
)

const wsUnsubscribeTimeout = 5 * time.Second

// WSError represents an error returned by the WebSocket API.
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

// WSEvent represents an event received from a subscription.
type WSEvent struct {
	SubscriptionID int64
	EventType      string
	Data           json.RawMessage
	Raw            json.RawMessage
}

// WSSubscription represents an active event subscription.
type WSSubscription struct {
	id     int64
	events chan WSEvent
	errors chan error
	client *WSClient
	once   sync.Once
}

// WSCallServiceResult represents a response to a call_service request.
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

// ReconnectConfig holds auto-reconnect settings.
type ReconnectConfig struct {
	Enabled     bool
	MaxRetries  int           // 0 = unlimited
	MinBackoff  time.Duration // default 1s
	MaxBackoff  time.Duration // default 60s
	OnReconnect func()        // called after successful reconnect
	OnError     func(error)   // called on each failed reconnect attempt
}

// subscriptionRequest stores info needed to restore a subscription.
type subscriptionRequest struct {
	request map[string]interface{}
	sub     *WSSubscription
}

// WSClient is a client for the Home Assistant WebSocket API.
type WSClient struct {
	config ClientConfig
	dialer *websocket.Dialer

	mu          sync.RWMutex
	conn        *websocket.Conn
	pending     map[int64]chan wsPendingResult
	pendingSubs map[int64]*WSSubscription
	subs        map[int64]*WSSubscription

	writeMu sync.Mutex
	nextID  int64

	// Reconnect state
	reconnect     ReconnectConfig
	activeSubs    map[int64]subscriptionRequest
	reconnecting  atomic.Bool
	stopReconnect chan struct{}
	closed        atomic.Bool
}

// WSOption is a function that configures the WSClient.
type WSOption func(*WSClient)

// WithAutoReconnect enables or disables automatic reconnection.
func WithAutoReconnect(enabled bool) WSOption {
	return func(c *WSClient) {
		c.reconnect.Enabled = enabled
	}
}

// WithMaxRetries sets the maximum number of reconnect attempts (0 = unlimited).
func WithMaxRetries(n int) WSOption {
	return func(c *WSClient) {
		c.reconnect.MaxRetries = n
	}
}

// WithReconnectBackoff sets the min and max backoff durations.
func WithReconnectBackoff(min, max time.Duration) WSOption {
	return func(c *WSClient) {
		c.reconnect.MinBackoff = min
		c.reconnect.MaxBackoff = max
	}
}

// WithOnReconnect sets a callback that is called after successful reconnect.
func WithOnReconnect(fn func()) WSOption {
	return func(c *WSClient) {
		c.reconnect.OnReconnect = fn
	}
}

// WithOnReconnectError sets a callback for failed reconnect attempts.
func WithOnReconnectError(fn func(error)) WSOption {
	return func(c *WSClient) {
		c.reconnect.OnError = fn
	}
}

// NewWSClient creates a new WebSocket client with optional configuration.
func NewWSClient(config ClientConfig, opts ...WSOption) *WSClient {
	c := &WSClient{
		config:        config,
		dialer:        websocket.DefaultDialer,
		activeSubs:    make(map[int64]subscriptionRequest),
		stopReconnect: make(chan struct{}),
		reconnect: ReconnectConfig{
			MinBackoff: time.Second,
			MaxBackoff: 60 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// SetLogger sets the debug logger for the WebSocket client.
func (c *WSClient) SetLogger(logger *slog.Logger) *WSClient {
	c.config.Logger = logger
	return c
}

// WS creates a new WebSocket client from the REST client configuration.
func (c *Client) WS(opts ...WSOption) *WSClient {
	return NewWSClient(c.config, opts...)
}

// Connect establishes the WebSocket connection; call it once and avoid concurrent calls.
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
	c.pendingSubs = make(map[int64]*WSSubscription)
	c.subs = make(map[int64]*WSSubscription)
	atomic.StoreInt64(&c.nextID, 0)
	c.mu.Unlock()

	go c.readLoop(conn)
	return nil
}

func (c *WSClient) Close() error {
	// Mark as closed to prevent reconnect
	if c.closed.Swap(true) {
		return nil // already closed
	}

	// Stop any reconnect loop
	select {
	case <-c.stopReconnect:
		// already closed
	default:
		close(c.stopReconnect)
	}

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
	c.pendingSubs[id] = sub
	c.mu.Unlock()

	if err := c.writeJSON(payload); err != nil {
		c.cleanupPending(id)
		c.mu.Lock()
		delete(c.pendingSubs, id)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pendingSubs, id)
		c.mu.Unlock()
		go c.unsubscribeIfCreated(respCh, id)
		return nil, ctx.Err()
	case res := <-respCh:
		if res.err != nil {
			return nil, res.err
		}
		if res.msg.Type != "result" || !res.msg.Success {
			if res.msg.Error != nil {
				return nil, res.msg.Error
			}
			return nil, errors.New("ws subscribe failed")
		}

		// Track subscription for auto-reconnect
		c.mu.Lock()
		c.activeSubs[sub.id] = subscriptionRequest{
			request: cloneWSRequest(req),
			sub:     sub,
		}
		c.mu.Unlock()

		return sub, nil
	}
}

func (c *WSClient) unsubscribe(ctx context.Context, id int64) error {
	// Remove from tracking first
	c.mu.Lock()
	delete(c.activeSubs, id)
	c.mu.Unlock()

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
			// Connection lost
			c.mu.Lock()
			c.conn = nil
			c.mu.Unlock()

			// Explicitly close the connection to avoid leaks
			_ = conn.Close()

			if c.closed.Load() {
				c.failAll(ErrWSClosed)
				return
			}

			if c.reconnect.Enabled {
				// Fail any pending requests (they won't be valid on new connection)
				c.failPending(ErrWSClosed)
				go c.reconnectLoop()
				return
			}

			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) || errors.Is(err, io.EOF) {
				c.failAll(ErrWSClosed)
				return
			}
			c.failAll(err)
			return
		}

		if isDebugEnabled(c.config.Logger, context.Background()) {
			c.config.Logger.Debug("recv", "payload", formatWSLogPayload(msg))
		}

		switch msg.Type {

		case "result", "pong":
			c.dispatchPending(msg)
		case "event":
			c.dispatchEvent(msg)
		}
	}
}

// failPending cancels all pending requests but leaves subscriptions intact.
func (c *WSClient) failPending(err error) {
	c.mu.Lock()
	pending := c.pending
	c.pending = make(map[int64]chan wsPendingResult)
	pendingSubs := c.pendingSubs
	c.pendingSubs = make(map[int64]*WSSubscription)
	c.mu.Unlock()

	for _, ch := range pending {
		ch <- wsPendingResult{err: err}
		close(ch)
	}
	// Also fail in-flight subscription requests
	for _, sub := range pendingSubs {
		sub.once.Do(func() {
			close(sub.events)
			close(sub.errors)
		})
	}
}

func (c *WSClient) dispatchPending(msg wsIncomingMessage) {
	var (
		sub       *WSSubscription
		resultErr error
	)
	c.mu.Lock()
	ch, ok := c.pending[msg.ID]
	if ok {
		delete(c.pending, msg.ID)
	}
	sub = c.pendingSubs[msg.ID]
	if sub != nil {
		delete(c.pendingSubs, msg.ID)
	}
	c.mu.Unlock()

	if !ok {
		return
	}
	if sub != nil && msg.Type == "result" && msg.Success {
		subscriptionID, err := subscriptionIDFromResult(msg.Result, msg.ID)
		if err != nil {
			resultErr = err
		} else {
			sub.id = subscriptionID
			c.mu.Lock()
			c.subs[subscriptionID] = sub
			c.mu.Unlock()
		}
	}
	ch <- wsPendingResult{msg: msg, err: resultErr}
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
	pendingSubs := c.pendingSubs
	c.pendingSubs = make(map[int64]*WSSubscription)
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
	for _, sub := range pendingSubs {
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
	if isDebugEnabled(c.config.Logger, context.Background()) {
		c.config.Logger.Debug("send", "payload", formatWSLogPayload(v))
	}
	return conn.WriteJSON(v)
}

// calculateBackoff returns the backoff duration for the given attempt.
func (c *WSClient) calculateBackoff(attempt int) time.Duration {
	backoff := c.reconnect.MinBackoff * time.Duration(1<<uint(attempt))
	if backoff > c.reconnect.MaxBackoff {
		backoff = c.reconnect.MaxBackoff
	}
	// Add jitter (±25%)
	jitter := time.Duration(rand.Int64N(int64(backoff)/2)) - backoff/4
	return backoff + jitter
}

// reconnectLoop attempts to reconnect with exponential backoff.
func (c *WSClient) reconnectLoop() {
	if !c.reconnecting.CompareAndSwap(false, true) {
		return // already reconnecting
	}
	defer c.reconnecting.Store(false)

	for attempt := 0; ; attempt++ {
		if c.closed.Load() {
			return
		}

		if c.reconnect.MaxRetries > 0 && attempt >= c.reconnect.MaxRetries {
			c.failAll(errors.New("max reconnect attempts exceeded"))
			return
		}

		backoff := c.calculateBackoff(attempt)

		select {
		case <-c.stopReconnect:
			return
		case <-time.After(backoff):
		}

		if c.closed.Load() {
			return
		}

		// Try to connect with timeout
		connectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := c.doConnect(connectCtx)
		cancel()
		if err != nil {
			if c.reconnect.OnError != nil {
				c.reconnect.OnError(err)
			}
			continue
		}

		// Success - restore subscriptions
		c.restoreSubscriptions()
		if c.reconnect.OnReconnect != nil {
			c.reconnect.OnReconnect()
		}
		return
	}
}

// doConnect performs the actual connection without checking existing connection.
func (c *WSClient) doConnect(ctx context.Context) error {
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
	c.pendingSubs = make(map[int64]*WSSubscription)
	c.subs = make(map[int64]*WSSubscription)
	atomic.StoreInt64(&c.nextID, 0)
	c.mu.Unlock()

	go c.readLoop(conn)
	return nil
}

// restoreSubscriptions re-subscribes all tracked subscriptions after reconnect.
func (c *WSClient) restoreSubscriptions() {
	c.mu.RLock()
	subs := make(map[int64]subscriptionRequest, len(c.activeSubs))
	for id, req := range c.activeSubs {
		subs[id] = req
	}
	c.mu.RUnlock()

	// Use a global timeout for restoration to avoid blocking forever
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for oldID, subReq := range subs {
		// Create new subscription
		newSub, err := c.subscribe(ctx, subReq.request)
		if err != nil {
			select {
			case subReq.sub.errors <- fmt.Errorf("failed to restore subscription: %w", err):
			default:
			}
			continue
		}

		// map the new ID to the OLD subscription object so the user keeps receiving events
		// map the new ID to the OLD subscription object so the user keeps receiving events
		c.mu.Lock()

		// If the new ID is different from the old ID, we need to clean up the old ID from activeSubs and subs
		if newSub.id != oldID {
			delete(c.activeSubs, oldID)
			delete(c.subs, oldID) // Clean up stale entry in main subs map
		}

		// Remove the temporary newSub created by subscribe() from activeSubs
		// We only want the OLD sub object in activeSubs, updated with the new ID
		delete(c.activeSubs, newSub.id)

		// Update the activeSubs with new ID but OLD sub object
		c.activeSubs[newSub.id] = subscriptionRequest{
			request: subReq.request,
			sub:     subReq.sub,
		}

		// Point the main subs map to the OLD sub object associated with the new ID
		c.subs[newSub.id] = subReq.sub

		// Update ID on the old sub object
		subReq.sub.id = newSub.id
		c.mu.Unlock()

		// Helper to forward events from newSub to oldSub (handling race condition where events arrive before swap)
		go func(src *WSSubscription, dst *WSSubscription) {
			// Wait a tiny bit to ensure any in-flight dispatch is done
			time.Sleep(100 * time.Millisecond)

			eventsCh := src.events
			errorsCh := src.errors
			for eventsCh != nil || errorsCh != nil {
				select {
				case ev, ok := <-eventsCh:
					if !ok {
						eventsCh = nil
						continue
					}
					// Fix subscription ID in the event to match the restored ID
					ev.SubscriptionID = dst.id

					select {
					case dst.events <- ev:
					default:
						// If destination full, we drop
					}
				case err, ok := <-errorsCh:
					if !ok {
						errorsCh = nil
						continue
					}
					if err == nil {
						continue
					}
					select {
					case dst.errors <- err:
					default:
					}
				default:
					return
				}
			}
		}(newSub, subReq.sub)
	}
}

func (c *WSClient) unsubscribeIfCreated(respCh <-chan wsPendingResult, fallbackID int64) {
	res, ok := <-respCh
	if !ok || res.err != nil {
		return
	}
	if res.msg.Type != "result" || !res.msg.Success {
		return
	}
	subscriptionID, err := subscriptionIDFromResult(res.msg.Result, fallbackID)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), wsUnsubscribeTimeout)
	defer cancel()
	_ = c.unsubscribe(ctx, subscriptionID)
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

func subscriptionIDFromResult(result json.RawMessage, fallbackID int64) (int64, error) {
	trimmed := bytes.TrimSpace(result)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return fallbackID, nil
	}
	var id int64
	if err := json.Unmarshal(trimmed, &id); err != nil {
		return 0, fmt.Errorf("invalid subscription id: %w", err)
	}
	return id, nil
}

func cloneWSRequest(req map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(req)+1)
	for k, v := range req {
		out[k] = v
	}
	return out
}
