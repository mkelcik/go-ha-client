# Examples

Ready-to-run examples for `go-ha-client`.

## Prerequisites
- Open the example `main.go` you want to run.
- Update constants at the top of the file (`haHost`, `haToken`, and entity/action constants).
- Use your Home Assistant URL and long-lived token.

## Run an example

```bash
go run ./examples/rest_ping
```

## REST examples

- `examples/rest_ping`: Ping Home Assistant and print basic config info.
- `examples/rest_switch_control`: Turn on/off/toggle a switch over REST using `CallServiceForEntity`.
- `examples/rest_camera_snapshot`: Download camera JPEG and save it to file.
- `examples/rest_render_template`: Render a Home Assistant template.
- `examples/rest_history_query`: Query entity history with `HistoryQuery` builder.

## WebSocket examples

- `examples/ws_switch_control`: Turn on/off/toggle a switch over WebSocket.
- `examples/ws_watch_light_state`: Watch `state_changed` events for one light.
- `examples/ws_wait_for_state`: Wait until an entity reaches target state (`WaitForState` / `WaitForStateEquals`).
- `examples/ws_auto_reconnect`: Subscribe to events with auto-reconnect callbacks.
- `examples/ws_call_service_for_entity`: Use helper `CallServiceForEntity` with brightness.

## Helper examples

- `examples/helpers_light_commands`: Use helper functions (`BuildEntityID`, `ParseEntityID`, `NewTurnLightOnCmd`, `NewTurnLightOffCmd`, `NewToggleLightCmd`, `NewServiceDataEntityID`).
- `examples/helpers_decode_event_data`: Offline helper demo for generic `DecodeEventData`.

## Notes
- `examples/helpers_decode_event_data` is offline and does not require Home Assistant connection.
- All other examples require valid Home Assistant host/token constants.
