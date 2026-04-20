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
- `examples/rest_ping_defaults`: Ping Home Assistant using `NewClientWithDefaults`.
- `examples/rest_switch_control`: Turn on/off/toggle a switch over REST using `CallServiceForEntity`.
- `examples/rest_camera_snapshot`: Download camera JPEG and save it to file.
- `examples/rest_render_template`: Render a Home Assistant template.
- `examples/rest_history_query`: Query entity history with `HistoryQuery` builder.

## WebSocket examples

- `examples/ws_switch_control`: Turn on/off/toggle a switch over WebSocket.
- `examples/ws_watch_light_state`: Watch `state_changed` events for one light.
- `examples/ws_watch_selected_entities`: Watch `state_changed` events for multiple selected entities (`SubscribeStateChangedMany`).
- `examples/ws_wait_for_state`: Wait until an entity reaches target state with custom predicate (`WaitForState`).
- `examples/ws_wait_for_state_equals`: Wait until an entity reaches a concrete state string (`WaitForStateEquals`).
- `examples/ws_wait_for_state_in`: Wait until an entity reaches one of multiple states (`WaitForStateIn`).
- `examples/ws_auto_reconnect`: Subscribe to events with auto-reconnect callbacks.
- `examples/ws_call_service_for_entity`: Use helper `CallServiceForEntity` with brightness.
- `examples/ws_watch_call_service_events`: Watch `call_service` events and decode with `WSEvent.CallServiceEvent`.
- `examples/ws_panels`: List registered UI panels (`GetPanels`).
- `examples/ws_validate_config`: Validate trigger/condition/action configs (`ValidateConfig`).
- `examples/ws_entity_registry`: Fetch the lightweight entity registry for UI display (`ListEntityRegistryForDisplay`).
- `examples/ws_expose_entity`: List and set voice-assistant exposure (`ListExposedEntities`, `ExposeEntity`).
- `examples/ws_target_helpers`: Resolve targets and inspect applicable triggers/conditions/services (`ExtractFromTarget`, `GetTriggersForTarget`, `GetConditionsForTarget`, `GetServicesForTarget`).
- `examples/ws_supported_features`: Opt-in message coalescing via `DeclareSupportedFeatures`.

## Helper examples

- `examples/helpers_light_commands`: Use helper functions (`BuildEntityID`, `ParseEntityID`, `NewTurnOnCmd`, `NewTurnOffCmd`, `NewToggleCmd`, `NewTurnLightOnCmd`, `NewTurnLightOffCmd`, `NewToggleLightCmd`, `NewServiceDataEntityID`).
- `examples/helpers_decode_event_data`: Offline helper demo for generic `DecodeEventData`.

## Notes
- `examples/helpers_decode_event_data` is offline and does not require Home Assistant connection.
- All other examples require valid Home Assistant host/token constants.
- Use the exact Home Assistant `entity_id` in examples (for example `light.kitchen` or a longer generated ID like `light.light_janka_ambient_level_light_color_on_off`), not only the UI/friendly name shown on dashboards.
- You can find the exact `entity_id` in Home Assistant under `Developer Tools -> States` or `Settings -> Devices & Services -> Entities`.
- `WaitForState`, `WaitForStateEquals`, and `WaitForStateIn` can run in parallel from multiple goroutines after a single `ws.Connect`.
