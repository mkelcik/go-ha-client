# Release Notes

## v2.0.0

First stable `v2` release of `go-ha-client`.

### Highlights
- REST API client aligned with the Home Assistant REST docs.
- WebSocket client with subscriptions, `call_service`, and optional auto-reconnect.
- Helper APIs for common entity/service flows (`CallServiceForEntity`, wait-for-state helpers, command builders).
- Typed event helpers for `state_changed` and `call_service` payload decoding.
- Debug logging for REST/WS request-response flows with token redaction.
- Runnable examples for REST, WebSocket, reconnect, and helper usage.

### Stability and API Policy
- `v2.0.0` freezes the public API for the `v2` major version.
- Future `v2.x` releases will remain backward compatible (additive features and fixes only).
- Breaking changes will be reserved for a future major version.

### Upgrade Notes
- `v1` users: see [`MIGRATION.md`](MIGRATION.md) for module path and API changes.
- `v2.0.0-beta.x` users: no import path change is required (`github.com/mkelcik/go-ha-client/v2`).
- Existing v2 beta integrations should continue to work without code changes if they used documented APIs.

### Operational Notes
- WebSocket reconnect callbacks (`WithOnReconnect`, `WithOnReconnectError`) are synchronous/blocking and should stay lightweight.
- Home Assistant event payloads use the full `entity_id` (for example `light.some_generated_name`), not the dashboard-friendly display name.

### Verification Summary
- Unit tests and race tests cover REST and WebSocket flows, including subscription cancellation behavior.
- Manual smoke tests against a real Home Assistant instance verified REST ping/config/state reads, WebSocket `state_changed` subscriptions, `WaitForStateEquals`, and WebSocket `CallServiceForEntity`.

See `CHANGELOG.md` for the detailed change history.
