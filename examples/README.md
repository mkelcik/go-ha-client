# Examples

Ready-to-run examples for `go-ha-client`.

## Prerequisites

Set these environment variables first:

```bash
export HA_HOST="http://homeassistant.local:8123"
export HA_TOKEN="your-long-lived-token"
```

## Run an example

```bash
go run ./examples/rest_ping
```

## REST examples

- `examples/rest_ping`: Ping Home Assistant and print basic config info.
- `examples/rest_switch_control`: Turn on/off/toggle a switch over REST.
- `examples/rest_camera_snapshot`: Download camera JPEG and save it to file.
- `examples/rest_render_template`: Render a Home Assistant template.
- `examples/rest_history_query`: Query entity history with `HistoryQuery` builder.

## WebSocket examples

- `examples/ws_switch_control`: Turn on/off/toggle a switch over WebSocket.
- `examples/ws_watch_light_state`: Watch `state_changed` events for one light.
- `examples/ws_wait_for_state`: Wait until an entity reaches target state.
- `examples/ws_auto_reconnect`: Subscribe to events with auto-reconnect callbacks.
- `examples/ws_call_service_for_entity`: Use helper `CallServiceForEntity` with brightness.

## Example-specific environment variables

### `examples/rest_switch_control`
- `HA_SWITCH_ENTITY_ID` (required), e.g. `switch.kitchen`.
- `HA_SWITCH_ACTION` (optional): `on`, `off`, `toggle` (default `toggle`).

### `examples/rest_camera_snapshot`
- `HA_CAMERA_ENTITY_ID` (required), e.g. `camera.front_door`.
- `HA_CAMERA_OUTPUT` (optional, default `camera.jpg`).

### `examples/rest_render_template`
- `HA_TEMPLATE` (optional, default `{{ states('sun.sun') }}`).

### `examples/rest_history_query`
- `HA_ENTITY_ID` (required), e.g. `light.kitchen`.
- `HA_HISTORY_HOURS` (optional, default `24`).

### `examples/ws_switch_control`
- `HA_SWITCH_ENTITY_ID` (required).
- `HA_SWITCH_ACTION` (optional): `on`, `off`, `toggle` (default `toggle`).

### `examples/ws_watch_light_state`
- `HA_LIGHT_ENTITY_ID` (required).
- `HA_WATCH_TIMEOUT_SECONDS` (optional, default `120`).

### `examples/ws_wait_for_state`
- `HA_LIGHT_ENTITY_ID` (required).
- `HA_TARGET_STATE` (optional, default `on`).
- `HA_WAIT_TIMEOUT_SECONDS` (optional, default `120`).

### `examples/ws_auto_reconnect`
- `HA_EVENT_TYPE` (optional, default `state_changed`).

### `examples/ws_call_service_for_entity`
- `HA_LIGHT_ENTITY_ID` (required).
- `HA_BRIGHTNESS` (optional 0-255, default `200`).
