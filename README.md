# go-ha-client
[![CI](https://github.com/mkelcik/go-ha-client/actions/workflows/ci.yml/badge.svg)](https://github.com/mkelcik/go-ha-client/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/mkelcik/go-ha-client/v2.svg)](https://pkg.go.dev/github.com/mkelcik/go-ha-client/v2)
[![Go Report Card](https://goreportcard.com/badge/github.com/mkelcik/go-ha-client/v2)](https://goreportcard.com/report/github.com/mkelcik/go-ha-client/v2)

Go client for Home Assistant REST API.
This client targets the official REST API documentation:
https://developers.home-assistant.io/docs/api/rest

It also includes WebSocket API client helpers based on:
https://developers.home-assistant.io/docs/api/websocket

The client follows official Home Assistant REST and WebSocket API docs.

### Requirements
- Go `1.25.12+`
- Home Assistant with long-lived access token

The minimum Go version is a deliberate security-first choice: this project tracks the **oldest still-supported Go minor line**, and within that line always its **latest security patch**. Upstream Go officially supports the two most recent minor releases — anything below that no longer receives security fixes, so we won't drop under it. At the same time, older patch versions of the same minor line may carry unfixed CVEs in `net`, `net/http`, or the TLS stack — all of which this client uses on every request — so the patch component is bumped as soon as upstream ships a security release, not on a fixed cadence. CI runs `govulncheck` to keep this honest. See [`SECURITY.md`](SECURITY.md) for the reporting process.

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

### Stability and Support Policy (v2)
- `v2.0.0` is the first stable v2 release and freezes the public API for the `v2` major line.
- `v2.x` releases may add new helpers/options and bug fixes, but will not introduce intentional breaking API changes.
- Breaking API changes will be introduced only in a new major version (for example `v3`).
- The package targets official Home Assistant REST and WebSocket APIs and follows their documented behavior where possible.
- Minimum supported Go version is `1.25.12+` (see CI/tooling in this repository). The minimum tracks the oldest still-supported Go minor line at its latest security patch — bumps are security-driven, not cosmetic, and may land in patch releases (see Requirements above).
- Security issues should be reported using the process in [`SECURITY.md`](SECURITY.md).

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

### Quick start
```go
package main

import (
	"context"
	"fmt"
	"time"

	ha "github.com/mkelcik/go-ha-client/v2"
)

func main() {
	client, err := ha.NewClient(
		"http://homeassistant.local:8123",
		ha.WithToken("YOUR_LONG_LIVED_TOKEN"),
		ha.WithTimeout(30*time.Second),
	)
	if err != nil {
		panic(err)
	}

	if err := client.Ping(context.Background()); err != nil {
		panic(err)
	}

	cfg, err := client.GetConfig(context.Background())
	if err != nil {
		panic(err)
	}
	fmt.Printf("Connected to Home Assistant %s\n", cfg.Version)
}
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

WebSocket command coverage (added in v2.2.0):
- [`examples/ws_panels`](examples/ws_panels) - list registered UI panels with `GetPanels`.
- [`examples/ws_validate_config`](examples/ws_validate_config) - validate trigger/condition/action configs with `ValidateConfig` (valid and invalid cases).
- [`examples/ws_entity_registry`](examples/ws_entity_registry) - fetch the UI-optimised entity registry with `ListEntityRegistryForDisplay`.
- [`examples/ws_expose_entity`](examples/ws_expose_entity) - list and set voice-assistant exposure with `ListExposedEntities` / `ExposeEntity`.
- [`examples/ws_target_helpers`](examples/ws_target_helpers) - resolve targets and inspect applicable triggers/conditions/services.
- [`examples/ws_supported_features`](examples/ws_supported_features) - opt-in message coalescing via `DeclareSupportedFeatures`.

The `examples` folder also includes:
- Template rendering
- History query builder usage
- Wait-for-state helper usage (`WaitForState`, `WaitForStateEquals`, and `WaitForStateIn`)
- Auto-reconnect WebSocket setup
- CallServiceForEntity helper usage (REST and WebSocket)
- State-changed subscription helpers for single or multiple entities
- Typed call_service event decoding helper usage

### Supported WebSocket commands
The client exposes typed helpers for every command documented in the official
[WebSocket API docs](https://developers.home-assistant.io/docs/api/websocket):

| Command | Method |
|---|---|
| `auth` / `auth_ok` / `auth_invalid` | `WSClient.Connect` |
| `ping` / `pong` | `WSClient.Ping` |
| `supported_features` | `WSClient.DeclareSupportedFeatures` (opt-in) |
| `get_states` | `WSClient.GetStates` |
| `get_config` | `WSClient.GetConfig` |
| `get_services` | `WSClient.GetServices` |
| `get_panels` | `WSClient.GetPanels` |
| `fire_event` | `WSClient.FireEvent` |
| `call_service` | `WSClient.CallService`, `WSClient.CallServiceWithResponse` |
| `subscribe_events` | `WSClient.SubscribeEvents` |
| `subscribe_trigger` | `WSClient.SubscribeTrigger` |
| `unsubscribe_events` | `WSSubscription.Unsubscribe` |
| `validate_config` | `WSClient.ValidateConfig` |
| `extract_from_target` | `WSClient.ExtractFromTarget` |
| `get_triggers_for_target` | `WSClient.GetTriggersForTarget` |
| `get_conditions_for_target` | `WSClient.GetConditionsForTarget` |
| `get_services_for_target` | `WSClient.GetServicesForTarget` |
| `config/entity_registry/list_for_display` | `WSClient.ListEntityRegistryForDisplay` |
| `homeassistant/expose_entity/list` | `WSClient.ListExposedEntities` |
| `homeassistant/expose_entity` | `WSClient.ExposeEntity` |

Any additional or future commands can be sent directly via `WSClient.Do(ctx, req, &out)`.

### WebSocket notes
- `WithOnReconnect` and `WithOnReconnectError` callbacks are called synchronously from the reconnect loop.
- Keep those callbacks short and non-blocking (offload heavy work to another goroutine if needed).
- Use typed event helpers (`WSEvent.StateChanged`, `WSEvent.CallServiceEvent`) to avoid manual payload casting.
- Event payloads use the real Home Assistant `entity_id` (for example `light.light_janka_ambient_level_light_color_on_off`), not the shorter UI/friendly name you may see in cards.
- In Home Assistant, find the exact `entity_id` in `Developer Tools -> States` (field `entity_id`) or `Settings -> Devices & Services -> Entities` (entity detail view).

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
- `WaitForState`, `WaitForStateEquals`, and `WaitForStateIn` are safe to run concurrently from multiple goroutines on one connected WS client.
- Always unsubscribe when done (`defer sub.Unsubscribe(...)`).
- Auto-reconnect is opt-in (disabled by default).
- During auto-reconnect, subscriptions are restored and buffered errors may be forwarded (buffers can still drop when full).
- Reconnect callbacks (`WithOnReconnect`, `WithOnReconnectError`) are blocking/synchronous and should stay lightweight.
