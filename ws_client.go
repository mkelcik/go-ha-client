package go_ha_client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
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
	// ErrEmptyTarget indicates a target selector has no references and cannot be used.
	ErrEmptyTarget = errors.New("target selector must reference at least one entity/device/area/floor/label")
	// ErrEmptyAssistants indicates an expose_entity request with no target assistants.
	ErrEmptyAssistants = errors.New("expose_entity requires at least one assistant")
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

// StateChangedEventData represents the payload for state_changed events.
type StateChangedEventData struct {
	EntityID string `json:"entity_id"`
	NewState *State `json:"new_state"`
	OldState *State `json:"old_state"`
}

// CallServiceEventData represents the payload for call_service events.
type CallServiceEventData struct {
	Domain      string                 `json:"domain"`
	Service     string                 `json:"service"`
	ServiceData map[string]interface{} `json:"service_data"`
}

// StateChanged returns state_changed data when available.
// ok=false means this event doesn't carry state_changed data.
func (e WSEvent) StateChanged() (StateChangedEventData, bool, error) {
	if e.EventType != EventTypeStateChanged {
		return StateChangedEventData{}, false, nil
	}
	if len(e.Data) == 0 {
		return StateChangedEventData{}, false, nil
	}
	var data StateChangedEventData
	if err := json.Unmarshal(e.Data, &data); err != nil {
		return StateChangedEventData{}, false, err
	}
	if data.EntityID == "" || data.NewState == nil {
		return StateChangedEventData{}, false, nil
	}
	return data, true, nil
}

// CallServiceEvent returns call_service data when available.
// ok=false means this event doesn't carry call_service data.
func (e WSEvent) CallServiceEvent() (CallServiceEventData, bool, error) {
	if e.EventType != EventTypeCallService {
		return CallServiceEventData{}, false, nil
	}
	if len(e.Data) == 0 {
		return CallServiceEventData{}, false, nil
	}
	var data CallServiceEventData
	if err := json.Unmarshal(e.Data, &data); err != nil {
		return CallServiceEventData{}, false, err
	}
	if data.Domain == "" || data.Service == "" {
		return CallServiceEventData{}, false, nil
	}
	if data.ServiceData == nil {
		data.ServiceData = map[string]interface{}{}
	}
	return data, true, nil
}

// WSSubscription represents an active event subscription.
type WSSubscription struct {
	id       int64
	idSource *int64
	events   chan WSEvent
	errors   chan error
	client   *WSClient
	once     sync.Once
	chMu     sync.Mutex
	closed   bool
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
	if s.idSource != nil {
		return atomic.LoadInt64(s.idSource)
	}
	return atomic.LoadInt64(&s.id)
}

// Events returns a channel of subscription events.
func (s *WSSubscription) Events() <-chan WSEvent {
	return s.events
}

// Errors returns a channel of subscription errors.
// During auto-reconnect, errors from the temporary subscription may be forwarded.
func (s *WSSubscription) Errors() <-chan error {
	return s.errors
}

func (s *WSSubscription) Unsubscribe(ctx context.Context) error {
	return s.client.unsubscribe(ctx, s.ID())
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
	OnReconnect func()        // called synchronously after successful reconnect; slow callbacks block reconnect flow
	OnError     func(error)   // called synchronously on each failed reconnect attempt; slow callbacks block reconnect flow
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
	pendingConn *websocket.Conn
	pending     map[int64]chan wsPendingResult
	pendingSubs map[int64]*WSSubscription
	subs        map[int64]*WSSubscription

	connectMu sync.Mutex
	writeMu   sync.Mutex
	nextID    int64

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
		if min <= 0 {
			min = time.Second
		}
		if max <= 0 {
			max = 60 * time.Second
		}
		if max < min {
			max = min
		}
		c.reconnect.MinBackoff = min
		c.reconnect.MaxBackoff = max
	}
}

// WithOnReconnect sets a callback that is called synchronously after successful reconnect.
// Keep the callback non-blocking because it runs on the reconnect loop path.
func WithOnReconnect(fn func()) WSOption {
	return func(c *WSClient) {
		c.reconnect.OnReconnect = fn
	}
}

// WithOnReconnectError sets a callback that is called synchronously on failed reconnect attempts.
// Keep the callback non-blocking because it runs on the reconnect loop path.
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
	c.connectMu.Lock()
	defer c.connectMu.Unlock()

	return c.connectLocked(ctx)
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
	pendingConn := c.pendingConn
	c.conn = nil
	c.pendingConn = nil
	c.mu.Unlock()

	if conn != nil {
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
		_ = conn.Close()
	}
	if pendingConn != nil && pendingConn != conn {
		_ = pendingConn.Close()
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

// DeclareSupportedFeatures sends a supported_features message advertising
// optional client capabilities (for example {"coalesce_messages": 1}).
//
// It is opt-in and never sent automatically from Connect, so existing
// integrations keep an identical handshake sequence. Call it explicitly
// right after Connect if you need the features it unlocks.
//
// Home Assistant documents the first user message as id=1, but any
// incrementing id (auto-assigned by this client) is accepted in practice.
//
// See https://developers.home-assistant.io/docs/api/websocket for the
// current list of supported feature keys.
func (c *WSClient) DeclareSupportedFeatures(ctx context.Context, features map[string]interface{}) error {
	req := map[string]interface{}{
		"type": "supported_features",
	}
	if features != nil {
		req["features"] = features
	}
	return c.Do(ctx, req, nil)
}

// GetPanels returns the registered UI panels (get_panels).
func (c *WSClient) GetPanels(ctx context.Context) (Panels, error) {
	panels := Panels{}
	return panels, c.Do(ctx, map[string]interface{}{
		"type": "get_panels",
	}, &panels)
}

// ValidateConfig validates trigger/condition/action configurations against the
// running Home Assistant instance. All three sections are optional.
func (c *WSClient) ValidateConfig(ctx context.Context, cfg ValidateConfigRequest) (ValidateConfigResult, error) {
	result := ValidateConfigResult{}
	req := map[string]interface{}{
		"type": "validate_config",
	}
	if cfg.Trigger != nil {
		req["trigger"] = cfg.Trigger
	}
	if cfg.Condition != nil {
		req["condition"] = cfg.Condition
	}
	if cfg.Action != nil {
		req["action"] = cfg.Action
	}
	return result, c.Do(ctx, req, &result)
}

// ExtractFromTarget resolves a target selector to concrete entity/device/area/etc. ids.
// If expandGroup is true, group entities are expanded to their members.
func (c *WSClient) ExtractFromTarget(ctx context.Context, target TargetSelector, expandGroup bool) (ExtractFromTargetResult, error) {
	result := ExtractFromTargetResult{}
	if target.IsEmpty() {
		return result, ErrEmptyTarget
	}
	req := map[string]interface{}{
		"type":         "extract_from_target",
		"target":       target,
		"expand_group": expandGroup,
	}
	return result, c.Do(ctx, req, &result)
}

// GetTriggersForTarget returns triggers applicable to a target (get_triggers_for_target).
// Per docs, expand_group defaults to true for this command.
func (c *WSClient) GetTriggersForTarget(ctx context.Context, target TargetSelector, expandGroup bool) ([]TriggerInfo, error) {
	var result []TriggerInfo
	if target.IsEmpty() {
		return nil, ErrEmptyTarget
	}
	return result, c.Do(ctx, buildTargetRequest("get_triggers_for_target", target, expandGroup), &result)
}

// GetConditionsForTarget returns conditions applicable to a target.
func (c *WSClient) GetConditionsForTarget(ctx context.Context, target TargetSelector, expandGroup bool) ([]ConditionInfo, error) {
	var result []ConditionInfo
	if target.IsEmpty() {
		return nil, ErrEmptyTarget
	}
	return result, c.Do(ctx, buildTargetRequest("get_conditions_for_target", target, expandGroup), &result)
}

// GetServicesForTarget returns services applicable to a target.
func (c *WSClient) GetServicesForTarget(ctx context.Context, target TargetSelector, expandGroup bool) ([]ServiceTargetInfo, error) {
	var result []ServiceTargetInfo
	if target.IsEmpty() {
		return nil, ErrEmptyTarget
	}
	return result, c.Do(ctx, buildTargetRequest("get_services_for_target", target, expandGroup), &result)
}

// ListEntityRegistryForDisplay returns a lightweight entity registry dump
// optimised for UI display (short field names, disabled entities excluded).
func (c *WSClient) ListEntityRegistryForDisplay(ctx context.Context) (DisplayEntityRegistry, error) {
	result := DisplayEntityRegistry{}
	return result, c.Do(ctx, map[string]interface{}{
		"type": "config/entity_registry/list_for_display",
	}, &result)
}

// ListExposedEntities returns the voice-assistant exposure map for every entity
// (homeassistant/expose_entity/list).
func (c *WSClient) ListExposedEntities(ctx context.Context) (ExposedEntitiesResult, error) {
	result := ExposedEntitiesResult{}
	return result, c.Do(ctx, map[string]interface{}{
		"type": "homeassistant/expose_entity/list",
	}, &result)
}

// ExposeEntity sets voice-assistant exposure for one or more entities.
// assistants must contain at least one of "conversation", "cloud.alexa",
// "cloud.google_assistant"; entityIDs must be non-empty.
func (c *WSClient) ExposeEntity(ctx context.Context, req ExposeEntityRequest) error {
	if len(req.Assistants) == 0 {
		return ErrEmptyAssistants
	}
	if len(req.EntityIDs) == 0 {
		return ErrEmptyEntityID
	}
	return c.Do(ctx, map[string]interface{}{
		"type":          "homeassistant/expose_entity",
		"assistants":    req.Assistants,
		"entity_ids":    req.EntityIDs,
		"should_expose": req.ShouldExpose,
	}, nil)
}

func buildTargetRequest(commandType string, target TargetSelector, expandGroup bool) map[string]interface{} {
	return map[string]interface{}{
		"type":         commandType,
		"target":       target,
		"expand_group": expandGroup,
	}
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
	if req == nil || req["type"] == "" {
		return nil, ErrWSInvalidRequest
	}

	sub := &WSSubscription{
		events: make(chan WSEvent, 32),
		errors: make(chan error, 1),
		client: c,
	}
	if err := c.subscribeWithSub(ctx, req, sub); err != nil {
		return nil, err
	}
	return sub, nil
}

func (c *WSClient) subscribeWithSub(ctx context.Context, req map[string]interface{}, sub *WSSubscription) error {
	id := atomic.AddInt64(&c.nextID, 1)
	if req == nil || req["type"] == "" {
		return ErrWSInvalidRequest
	}

	if atomic.LoadInt64(&sub.id) == 0 {
		atomic.StoreInt64(&sub.id, id)
	}

	respCh := make(chan wsPendingResult, 1)
	payload := cloneWSRequest(req)
	payload["id"] = id

	c.mu.Lock()
	if c.conn == nil {
		c.mu.Unlock()
		return ErrWSNotConnected
	}
	c.pending[id] = respCh
	c.pendingSubs[id] = sub
	c.mu.Unlock()

	if err := c.writeJSON(payload); err != nil {
		c.cleanupPending(id)
		c.mu.Lock()
		delete(c.pendingSubs, id)
		c.mu.Unlock()
		return err
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pendingSubs, id)
		c.mu.Unlock()
		go c.unsubscribeIfCreated(respCh, id)
		return ctx.Err()
	case res := <-respCh:
		if res.err != nil {
			return res.err
		}
		if res.msg.Type != "result" || !res.msg.Success {
			if res.msg.Error != nil {
				return res.msg.Error
			}
			return errors.New("ws subscribe failed")
		}

		// If the context was canceled while the subscribe result was in flight,
		// prefer the cancellation outcome and best-effort unsubscribe. This
		// avoids nondeterministic select behavior when ctx.Done() and respCh are
		// both ready at the same time.
		if err := ctx.Err(); err != nil {
			if subscriptionID, subErr := subscriptionIDFromResult(res.msg.Result, id); subErr == nil {
				go func() {
					unsubCtx, cancel := context.WithTimeout(context.Background(), wsUnsubscribeTimeout)
					defer cancel()
					_ = c.unsubscribe(unsubCtx, subscriptionID)
				}()
			}
			return err
		}

		// Track subscription for auto-reconnect
		c.mu.Lock()
		c.activeSubs[sub.ID()] = subscriptionRequest{
			request: cloneWSRequest(req),
			sub:     sub,
		}
		c.mu.Unlock()

		return nil
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

	failPendingResponses(pending, err)
	closeSubscriptionSet(pendingSubs, nil)
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
			atomic.StoreInt64(&sub.id, subscriptionID)
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

	if trySendSubscriptionEvent(sub, event) {
		return
	}
	notifySubscriptionError(sub, errors.New("ws subscription event buffer is full"))
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
	notifySubscriptionError(sub, reportErr)
	if closeChannels {
		closeSubscriptionChannels(sub)
	}
}

func (c *WSClient) failAll(err error) {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	pendingConn := c.pendingConn
	c.pendingConn = nil
	pending := c.pending
	c.pending = make(map[int64]chan wsPendingResult)
	pendingSubs := c.pendingSubs
	c.pendingSubs = make(map[int64]*WSSubscription)
	subs := c.subs
	c.subs = make(map[int64]*WSSubscription)
	c.activeSubs = make(map[int64]subscriptionRequest)
	c.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	if pendingConn != nil && pendingConn != conn {
		_ = pendingConn.Close()
	}

	failPendingResponses(pending, err)
	closeSubscriptionSet(pendingSubs, err)
	closeSubscriptionSet(subs, err)
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
	if attempt < 0 {
		attempt = 0
	}
	backoff := c.reconnect.MinBackoff
	for i := 0; i < attempt; i++ {
		if backoff >= c.reconnect.MaxBackoff/2 {
			backoff = c.reconnect.MaxBackoff
			break
		}
		backoff *= 2
	}
	if backoff > c.reconnect.MaxBackoff {
		backoff = c.reconnect.MaxBackoff
	}
	// Add jitter (±25%)
	jitter := time.Duration(cryptoInt63n(int64(backoff)/2)) - backoff/4
	return backoff + jitter
}

func cryptoInt63n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(n))
	if err != nil {
		return 0
	}
	return v.Int64()
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
			c.invokeReconnectError(err)
			continue
		}

		// Success - restore subscriptions
		c.restoreSubscriptions()
		c.invokeReconnected()
		return
	}
}

func (c *WSClient) invokeReconnected() {
	if c.reconnect.OnReconnect == nil {
		return
	}
	defer c.recoverReconnectCallbackPanic("on_reconnect")
	c.reconnect.OnReconnect()
}

func (c *WSClient) invokeReconnectError(err error) {
	if c.reconnect.OnError == nil {
		return
	}
	defer c.recoverReconnectCallbackPanic("on_reconnect_error")
	c.reconnect.OnError(err)
}

func (c *WSClient) recoverReconnectCallbackPanic(callback string) {
	recovered := recover()
	if recovered == nil {
		return
	}

	if c.config.Logger != nil {
		c.config.Logger.Error("ws reconnect callback panic recovered", "callback", callback, "panic", recovered)
	}
}

// doConnect is used by reconnect flow and shares the same connect path as Connect.
func (c *WSClient) doConnect(ctx context.Context) error {
	c.connectMu.Lock()
	defer c.connectMu.Unlock()

	return c.connectLocked(ctx)
}

// connectLocked performs dial+auth+attach; caller must hold connectMu.
func (c *WSClient) connectLocked(ctx context.Context) error {
	if c.closed.Load() {
		return ErrWSClosed
	}
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
	c.mu.Lock()
	if c.closed.Load() {
		c.mu.Unlock()
		_ = conn.Close()
		return ErrWSClosed
	}
	c.pendingConn = conn
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		if c.pendingConn == conn {
			c.pendingConn = nil
		}
		c.mu.Unlock()
	}()

	msg := wsIncomingMessage{}
	if err := conn.ReadJSON(&msg); err != nil {
		_ = conn.Close()
		if c.closed.Load() {
			return ErrWSClosed
		}
		return err
	}
	if msg.Type != "auth_required" {
		_ = conn.Close()
		if c.closed.Load() {
			return ErrWSClosed
		}
		return fmt.Errorf("unexpected ws handshake message: %s", msg.Type)
	}

	if err := conn.WriteJSON(map[string]interface{}{
		"type":         "auth",
		"access_token": c.config.Token,
	}); err != nil {
		_ = conn.Close()
		if c.closed.Load() {
			return ErrWSClosed
		}
		return err
	}

	msg = wsIncomingMessage{}
	if err := conn.ReadJSON(&msg); err != nil {
		_ = conn.Close()
		if c.closed.Load() {
			return ErrWSClosed
		}
		return err
	}
	if msg.Type != "auth_ok" {
		_ = conn.Close()
		if c.closed.Load() {
			return ErrWSClosed
		}
		if msg.Type == "auth_invalid" {
			return fmt.Errorf("%w: %s", ErrWSAuthFailed, msg.Message)
		}
		return fmt.Errorf("unexpected ws auth message: %s", msg.Type)
	}

	c.mu.Lock()
	if c.closed.Load() {
		c.mu.Unlock()
		_ = conn.Close()
		return ErrWSClosed
	}
	if c.conn != nil {
		c.mu.Unlock()
		_ = conn.Close()
		return nil
	}
	c.pendingConn = nil
	c.conn = conn
	c.pending = make(map[int64]chan wsPendingResult)
	c.pendingSubs = make(map[int64]*WSSubscription)
	c.subs = make(map[int64]*WSSubscription)
	atomic.StoreInt64(&c.nextID, 0)
	c.mu.Unlock()

	go c.readLoop(conn)
	return nil
}

func failPendingResponses(pending map[int64]chan wsPendingResult, err error) {
	for _, ch := range pending {
		ch <- wsPendingResult{err: err}
		close(ch)
	}
}

func notifySubscriptionError(sub *WSSubscription, err error) {
	if sub == nil || err == nil {
		return
	}
	sub.chMu.Lock()
	defer sub.chMu.Unlock()
	if sub.closed {
		return
	}
	select {
	case sub.errors <- err:
	default:
	}
}

func trySendSubscriptionEvent(sub *WSSubscription, event WSEvent) bool {
	if sub == nil {
		return false
	}
	sub.chMu.Lock()
	defer sub.chMu.Unlock()
	if sub.closed {
		return false
	}
	select {
	case sub.events <- event:
		return true
	default:
		return false
	}
}

func closeSubscriptionChannels(sub *WSSubscription) {
	if sub == nil {
		return
	}
	sub.chMu.Lock()
	defer sub.chMu.Unlock()
	if sub.closed {
		return
	}
	sub.closed = true
	close(sub.events)
	close(sub.errors)
}

func closeSubscriptionSet(subs map[int64]*WSSubscription, err error) {
	for _, sub := range subs {
		notifySubscriptionError(sub, err)
		closeSubscriptionChannels(sub)
	}
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
		if err := c.subscribeWithSub(ctx, subReq.request, subReq.sub); err != nil {
			notifySubscriptionError(subReq.sub, fmt.Errorf("failed to restore subscription: %w", err))
			continue
		}

		newID := subReq.sub.ID()
		c.mu.Lock()
		if newID != oldID {
			delete(c.activeSubs, oldID)
			delete(c.subs, oldID)
		}
		c.activeSubs[newID] = subscriptionRequest{
			request: subReq.request,
			sub:     subReq.sub,
		}
		c.subs[newID] = subReq.sub
		c.mu.Unlock()
	}
}

func (c *WSClient) unsubscribeIfCreated(respCh <-chan wsPendingResult, fallbackID int64) {
	timer := time.NewTimer(wsUnsubscribeTimeout)
	defer timer.Stop()

	select {
	case res, ok := <-respCh:
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
	case <-timer.C:
		c.cleanupPending(fallbackID)
		return
	}
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
