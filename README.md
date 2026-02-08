# go-ha-client
Go client for Home Assistant REST API.
This client targets the official REST API documentation:
https://developers.home-assistant.io/docs/api/rest

It also includes WebSocket API client helpers based on:
https://developers.home-assistant.io/docs/api/websocket

Tested with home-assistant `core-2021.7.2`.

### Requirements
- Go `1.24.13+`
- Home Assistant with long-lived access token

### Get a Home Assistant token
1. Open Home Assistant in the browser.
2. Click your user profile (bottom-left).
3. Scroll to **Long-Lived Access Tokens**.
4. Click **Create Token**, give it a name, and copy the value.

### Project docs
- [Contributing guide](CONTRIBUTING.md)
- [Security policy](SECURITY.md)
- [Code of conduct](CODE_OF_CONDUCT.md)
- [Changelog](CHANGELOG.md)
- [Release notes](RELEASE_NOTES.md)
- [Migration guide](MIGRATION.md)

### Production readiness checklist (beta)
- Freeze the public API and tag `v2.0.0` (no breaking changes after that).
- Document stability/support policy in README.
- Add 1-2 WS integration tests (reconnect + subscribe + call_service end-to-end).
- Prepare release notes and a short migration guide for v2.

### Install
```bash
go get github.com/mkelcik/go-ha-client/v2@latest
```

### REST vs WebSocket
- Use **REST** for request/response operations (query state, call service, render template).
- Use **WebSocket** for realtime subscriptions (`state_changed`, triggers, live event stream).


### Basic usage
Change `Token` and `Host` to your actual home-assistant bearer token and address
Check home-assistant documentation how get access token.

```go
package main

import (
	"context"
	"fmt"
	ha "github.com/mkelcik/go-ha-client/v2"
	"net/http"
	"time"
)

func main() {
	client, err := ha.NewClient("http://my-ha.home", 
		ha.WithToken("mytoken"),
		ha.WithHTTPClient(&http.Client{
			Timeout: 30 * time.Second,
		}),
	)
	if err != nil {
		panic(err)
	}
    
	// ping instance
	if err := client.Ping(context.Background()); err != nil {
		fmt.Println("connection error", err)
	} else {
		fmt.Println("connection ok")
	}

	// example of home-assistant instance info
	cfg, err := client.GetConfig(context.Background())
	if err != nil {
		panic(err)
	}
	fmt.Printf("%+v", cfg)
}
```

### Debug logging
Enable debug logs for REST and WebSocket requests/responses:
```go
	client, err := ha.NewClient("http://ha.home",
		ha.WithToken("token"),
		ha.WithDebug(),
		ha.WithHTTPClient(&http.Client{
			Timeout: 30 * time.Second,
		}),
	)
	if err != nil {
		panic(err)
	}
```
When enabled, request/response bodies are logged (tokens are redacted for WS auth).

### Examples

To turn light with entity id `light.light_1` on, we can use `NewTurnLightOnCmd` helper, to create command and call service.
```go
// turn light on
if _, err := client.CallService(context.Background(), ha.NewTurnLightOnCmd("light.light_1")); err != nil {
	panic(err)
}

// turn light off 
if _, err := client.CallService(context.Background(), ha.NewTurnLightOffCmd("light.light_1")); err != nil {
	panic(err)
}
```
or turn `switch.switch_1` off without helper
```go
if _, err := client.CallService(context.Background(), ha.DefaultServiceCmd{
    Service:  ha.ServiceTurnOff,
    Domain:   ha.DomainSwitch, 
    EntityID: "switch.switch_1",
}); err != nil {
	panic(err)
}
```

Take and save picture from camera 
```go
camImg, err := client.GetCameraJpeg(context.Background(), "camera.my_camera")
if err != nil {
	panic(err)
}

f, err := os.Create("camera.jpg")
if err != nil {
	panic(err)
}
defer f.Close()

if err := jpeg.Encode(f, camImg, nil); err != nil {
	panic(err)
}
```

### More examples

Get components
```go
components, err := client.GetComponents(context.Background())
if err != nil {
	panic(err)
}
fmt.Println(components)
```

Render template
```go
rendered, err := client.RenderTemplate(context.Background(), "{{ states('sensor.test') }}")
if err != nil {
	panic(err)
}
fmt.Println(rendered)
```

Calendar events
```go
start := time.Now().Add(-24 * time.Hour)
end := time.Now()
events, err := client.GetCalendarEvents(context.Background(), "calendar.home", start, end)
if err != nil {
	panic(err)
}
fmt.Println(events)
```

Handle intent
```go
resp, err := client.HandleIntent(context.Background(), ha.IntentRequest{
	Name: "HassTurnOn",
	Data: map[string]interface{}{"entity": "light.kitchen"},
})
if err != nil {
	panic(err)
}
fmt.Println(resp.Response)
```

Weather forecasts
```go
forecast, err := client.GetWeatherForecasts(context.Background(), "weather.home", "daily")
if err != nil {
	panic(err)
}
fmt.Println(forecast.Forecast)
```

### Helpers

Subscribe to state changes for a single entity
```go
sub, err := ws.SubscribeStateChanged(context.Background(), "light.kitchen")
if err != nil {
	panic(err)
}
defer sub.Unsubscribe(context.Background())
```

Wait for a specific state
```go
err := ws.WaitForState(context.Background(), "light.kitchen", func(s ha.State) bool {
	return s.State == "on"
})
if err != nil {
	panic(err)
}
```

Call a service with entity_id prefilled
```go
_, err := ws.CallServiceForEntity(
	context.Background(),
	"light",
	"turn_on",
	"light.kitchen",
	map[string]interface{}{"brightness": 200},
)
if err != nil {
	panic(err)
}
```

Decode event payloads
```go
type StateChanged struct {
	EntityID string  `json:"entity_id"`
	NewState ha.State `json:"new_state"`
}

ev := <-sub.Events()
data, err := ha.DecodeEventData[StateChanged](ev)
if err != nil {
	panic(err)
}
fmt.Println(data.EntityID, data.NewState.State)
```

Entity ID helpers
```go
id := ha.BuildEntityID("light", "kitchen") // light.kitchen
domain, objectID, err := ha.ParseEntityID(id)
if err != nil {
	panic(err)
}
fmt.Println(domain, objectID)
```

History query builder
```go
query := ha.NewHistoryQuery().
	WithStart(time.Now().Add(-24 * time.Hour)).
	WithEnd(time.Now()).
	WithEntities("light.kitchen", "sensor.temp").
	WithNoAttributes(true)

history, err := client.GetHistory(context.Background(), query)
if err != nil {
	panic(err)
}
fmt.Println(len(history))
```

### WebSocket Client
The client includes a WebSocket helper for real-time events.

#### Basic Usage
```go
// Create WebSocket client from REST client
ws := client.WS(
	ha.WithAutoReconnect(true), // Enable auto-reconnect
	ha.WithOnReconnect(func() {
		log.Println("Reconnected to Home Assistant")
	}),
)

if err := ws.Connect(context.Background()); err != nil {
	panic(err)
}
defer ws.Close()

// Subscribe to events
sub, err := ws.SubscribeEvents(context.Background(), "state_changed")
if err != nil {
	panic(err)
}

// Automatically restores subscription after reconnect!
for event := range sub.Events() {
	fmt.Printf("Event: %s\n", event.EventType)
}
```

#### Auto-Reconnect Configuration
The WebSocket client supports automatic reconnection with exponential backoff.
```go
ws := client.WS(
	ha.WithAutoReconnect(true),
	ha.WithMaxRetries(10),           // Unlimited if 0
	ha.WithReconnectBackoff(time.Second, 60*time.Second),
	ha.WithOnReconnect(func() { ... }),
	ha.WithOnReconnectError(func(err error) { ... }),
)
```

#### Advanced Usage

**Parse events:**
```go
type StateChangedEvent struct {
	EntityID string `json:"entity_id"`
	NewState struct {
		State string `json:"state"`
	} `json:"new_state"`
}

for ev := range sub.Events() {
	if ev.EventType != "state_changed" {
		continue
	}
	var data StateChangedEvent
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		continue
	}
	if data.EntityID == "light.kitchen" {
		fmt.Println("light.kitchen changed", data.NewState.State)
	}
}
```

**Call a service over WebSocket:**
```go
result, err := ws.CallService(
	context.Background(),
	"light",
	"turn_on",
	map[string]interface{}{"entity_id": "light.kitchen"},
)
if err != nil {
	panic(err)
}
fmt.Println("context id:", result.Context.ID)
```

### Error handling
Use sentinel errors with `errors.Is`:
```go
if _, err := client.GetStateForEntity(ctx, ""); errors.Is(err, ha.ErrEmptyEntityID) {
	fmt.Println("entity id is required")
}

if err := client.Ping(ctx); errors.Is(err, ha.ErrUnauthorized) {
	fmt.Println("token is invalid or expired")
}
```

### WebSocket lifecycle notes
- Open one WS connection and reuse it (`ws.Connect` once, `defer ws.Close()`).
- Always unsubscribe when done (`defer sub.Unsubscribe(...)`).
- Auto-reconnect is opt-in (disabled by default).
- During auto-reconnect, subscriptions are restored and buffered errors may be forwarded (buffers can still drop when full).
