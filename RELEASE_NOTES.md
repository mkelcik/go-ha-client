# Release Notes

## v2.1.0

Security and tooling maintenance release for `v2`.

### Highlights
- Bump minimum Go version, module toolchain, and CI runtime from `1.24.13` to `1.25.8`.
- Resolve `govulncheck` findings from Go standard library by running/building with patched Go release:
  - GO-2026-4601 (`net/url`)
  - GO-2026-4602 (`os`)

### Compatibility
- No public API changes.
- No import path changes (`github.com/mkelcik/go-ha-client/v2`).
- Existing `v2.x` integrations should continue to work without code changes.

### Upgrade Notes
- Ensure local/dev/CI environments use Go `1.25.8+`.
- If your pipeline pins Go patch versions, update them to `1.25.8` or newer.

### Verification Summary
- `go test ./...` passes.
- `go test -race ./...` passes.
- `govulncheck ./...` reports no reachable vulnerabilities on Go `1.25.8`.

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
