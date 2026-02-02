# go-ha-client
Go client for Home Assistant REST API.

This client targets the official REST API documentation:
https://developers.home-assistant.io/docs/api/rest

It also includes WebSocket API client helpers based on:
https://developers.home-assistant.io/docs/api/websocket

Tested with home-assistant `core-2021.7.2`.

### Requirements
- Go `1.24.12+`
- Home Assistant with long-lived access token

### Project docs
- [Contributing guide](CONTRIBUTING.md)
- [Security policy](SECURITY.md)
- [Code of conduct](CODE_OF_CONDUCT.md)
- [Changelog](CHANGELOG.md)

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
	client := ha.NewClient(ha.ClientConfig{Token: "mytoken", Host: "http://my-ha.home"}, &http.Client{
		Timeout: 30 * time.Second,
	})
    
	// ping instance
	if err := client.Ping(context.Background()); err != nil {
		fmt.Println("connection error", err)
	} else {
		fmt.Println("connection ok")
	}

	// example of home-assistant instance info
	discoverInfo, err := client.GetDiscoverInfo(context.Background())
	if err != nil {
		panic(err)
	}
	fmt.Printf("%+v", discoverInfo)
}
```

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
    EntityId: "switch.switch_1",
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

Conversation process
```go
conv, err := client.ProcessConversation(context.Background(), ha.ConversationProcessRequest{
	Text:     "Turn on kitchen lights",
	Language: "en",
})
if err != nil {
	panic(err)
}
fmt.Println(conv.Response)
```

Weather forecasts
```go
forecast, err := client.GetWeatherForecasts(context.Background(), "weather.home", "daily")
if err != nil {
	panic(err)
}
fmt.Println(forecast.Forecast)
```

### WebSocket examples

Connect and subscribe to state changes
```go
ws := client.WS()
if err := ws.Connect(context.Background()); err != nil {
	panic(err)
}
defer ws.Close()

sub, err := ws.SubscribeEvents(context.Background(), ha.EventTypeStateChanged)
if err != nil {
	panic(err)
}
defer sub.Unsubscribe(context.Background())

type StateChangedEvent struct {
	EntityID string `json:"entity_id"`
	NewState struct {
		State string `json:"state"`
	} `json:"new_state"`
}

for ev := range sub.Events() {
	if ev.EventType != ha.EventTypeStateChanged {
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

Call a service over WebSocket
```go
result, err := ws.CallService(
	context.Background(),
	ha.DomainLight,
	ha.ServiceTurnOn,
	ha.NewServiceDataEntityID("light.kitchen"),
)
if err != nil {
	panic(err)
}
fmt.Println("context id:", result.Context.ID)
```

Print message when a specific light turns on
```go
ws := client.WS()
if err := ws.Connect(context.Background()); err != nil {
	panic(err)
}
defer ws.Close()

sub, err := ws.SubscribeEvents(context.Background(), ha.EventTypeStateChanged)
if err != nil {
	panic(err)
}
defer sub.Unsubscribe(context.Background())

for ev := range sub.Events() {
	var data struct {
		EntityID string `json:"entity_id"`
		NewState struct {
			State string `json:"state"`
		} `json:"new_state"`
	}
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		continue
	}
	if data.EntityID == "light.kitchen" && data.NewState.State == "on" {
		fmt.Println("light.kitchen is ON")
	}
}
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
- Auto-reconnect is not built in yet; if connection drops, reconnect in your app loop.
