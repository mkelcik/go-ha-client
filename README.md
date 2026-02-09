# go-ha-client
Go client for Home Assistant REST API.
This client targets the official REST API documentation:
https://developers.home-assistant.io/docs/api/rest

It also includes WebSocket API client helpers based on:
https://developers.home-assistant.io/docs/api/websocket

The client follows official Home Assistant REST and WebSocket API docs.

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

### Non-goals
- Full coverage of all Home Assistant APIs (focus is official REST/WS docs).
- Automations/blueprints scheduling or orchestration.
- Deep schema validation for service payloads (delegated to HA).
- Token management or authentication flows (token must be provided).
- Strong typing of dynamic event attributes (kept as `map[string]interface{}`).
- Support for unofficial/custom endpoints.

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

Shortest setup:
```go
client, err := ha.NewClientWithDefaults("http://my-ha.home", "mytoken")
```

Configurable setup:
```go
package main

import (
	"context"
	"fmt"
	ha "github.com/mkelcik/go-ha-client/v2"
	"time"
)

func main() {
	client, err := ha.NewClient("http://my-ha.home", 
		ha.WithToken("mytoken"),
		ha.WithTimeout(30*time.Second), // Optional (default is 30s)
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
		ha.WithTimeout(30*time.Second),
	)
	if err != nil {
		panic(err)
	}
```
When enabled, request/response bodies are logged (tokens are redacted for WS auth).

### Examples
Ready-to-run examples for beginners are in [`examples/`](examples/README.md).

Recommended first runs:
- [`examples/rest_ping`](examples/rest_ping) - check connection and print instance info.
- [`examples/rest_ping_defaults`](examples/rest_ping_defaults) - shortest startup with `NewClientWithDefaults`.
- [`examples/rest_switch_control`](examples/rest_switch_control) - turn on/off/toggle a switch over REST.
- [`examples/ws_switch_control`](examples/ws_switch_control) - turn on/off/toggle a switch over WebSocket.
- [`examples/ws_watch_light_state`](examples/ws_watch_light_state) - watch light state changes in real time.
- [`examples/rest_camera_snapshot`](examples/rest_camera_snapshot) - fetch and save camera JPEG.
- [`examples/helpers_light_commands`](examples/helpers_light_commands) - quick tour of command/entity helper functions.
- [`examples/helpers_decode_event_data`](examples/helpers_decode_event_data) - simple `DecodeEventData` helper demo (offline).

The `examples` folder also includes:
- Template rendering
- History query builder usage
- Wait-for-state helper usage (`WaitForState` and `WaitForStateEquals`)
- Auto-reconnect WebSocket setup
- CallServiceForEntity helper usage (REST and WebSocket)
- State-changed subscription helpers for single or multiple entities
- Typed call_service event decoding helper usage

### WebSocket notes
- `WithOnReconnect` and `WithOnReconnectError` callbacks are called synchronously from the reconnect loop.
- Keep those callbacks short and non-blocking (offload heavy work to another goroutine if needed).
- Use typed event helpers (`WSEvent.StateChanged`, `WSEvent.CallServiceEvent`) to avoid manual payload casting.

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
- Reconnect callbacks (`WithOnReconnect`, `WithOnReconnectError`) are blocking/synchronous and should stay lightweight.
