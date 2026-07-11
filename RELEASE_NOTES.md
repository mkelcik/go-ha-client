# Release Notes

## v2.2.3

Security patch release: bumps the Go toolchain to `go1.25.12` to clear a `crypto/tls` standard-library vulnerability.

### Highlights
- Toolchain bumped `go1.25.10` → `go1.25.12` in `go.mod`. This picks up the upstream `crypto/tls` fix for [GO-2026-5856](https://pkg.go.dev/vuln/GO-2026-5856), which the nightly `govulncheck` job surfaced.
- The vulnerability is in the Go standard library (exercised on every HTTPS/WebSocket request), **not** in this project's code or its dependencies.
- Minimum supported Go version documented as `1.25.12+` in `README.md`, `CONTRIBUTING.md`, and `AGENTS.md`, consistent with the security-first version policy (track the oldest still-supported Go minor line at its latest security patch).

### Compatibility
- No public API changes.
- No upgrade steps required from `v2.2.2` beyond building with Go `1.25.12+`.

### Verification Summary
- Nightly `govulncheck` clears GO-2026-5856 under the bumped toolchain.

## v2.2.2

Documentation-only release: clarifies the rationale behind the minimum Go version.

### Highlights
- `README.md` `### Requirements` now explains that the minimum Go version (`1.25.10+`) is a deliberate security-first choice: the project tracks the **oldest still-supported Go minor line** at its **latest security patch**. Upstream Go only ships security fixes for the two most recent minor releases, and older patches of the same minor line may carry unfixed CVEs in `net`, `net/http`, or the TLS stack — all of which this client exercises on every request.
- Patch-level Go bumps may land in patch releases of this library (as `v2.1.1` and `v2.1.2` already did).
- CI continues to run `govulncheck` to enforce the policy.
- Matching one-liner added to `### Stability and Support Policy (v2)` so both sections tell the same story.

### Compatibility
- Purely additive documentation change.
- No code, no public API, no `go.mod` changes.
- No upgrade steps required from `v2.2.1`.

## v2.2.1

Documentation-only release: adds `AGENTS.md`, an integration guide tailored for AI coding agents using `github.com/mkelcik/go-ha-client/v2`.

### Highlights
- New `AGENTS.md` with install + import path, REST vs WebSocket decision table, minimal client snippets, full API cheat-sheet (`*Client`, `*WSClient`, helpers), common recipes, sentinel error handling, and a Gotchas section covering the most frequent integration mistakes.
- Subscription example uses `ev, ok := <-sub.Events()` and unsubscribes with a fresh context so cleanup survives loop-context cancellation.
- Go requirement (`1.25.10+`) aligned with `README.md`, `CONTRIBUTING.md`, and `go.mod`.

### Compatibility
- Purely additive documentation change.
- No code, no public API, no `go.mod` changes.
- No upgrade steps required from `v2.2.0`.

### Verification Summary
- CI green on the PR introducing `AGENTS.md` (`ci`, `mod_tidy_check`, `race`, `coverage`).

## v2.2.0

Parity release: the WebSocket client now exposes typed helpers for every command documented on the official [WebSocket API page](https://developers.home-assistant.io/docs/api/websocket).

### Highlights
- New typed methods on `WSClient`: `DeclareSupportedFeatures`, `GetPanels`, `ValidateConfig`, `ExtractFromTarget`, `GetTriggersForTarget`, `GetConditionsForTarget`, `GetServicesForTarget`, `ListEntityRegistryForDisplay`, `ListExposedEntities`, `ExposeEntity`.
- New public types for requests and responses (see `CHANGELOG.md`).
- New sentinel errors `ErrEmptyTarget` (empty `TargetSelector` inputs) and `ErrEmptyAssistants` (empty assistants slice in `ExposeEntity`).
- Six new runnable examples under `examples/` covering every new method.
- REST API coverage confirmed to be 100% against the official REST docs; no REST changes.

### Compatibility
- Purely additive. No public method signatures changed, no existing types changed.
- `Connect()` handshake is unchanged; `supported_features` remains opt-in via `DeclareSupportedFeatures` so existing integrations behave identically.
- `WSClient.Do(ctx, req, out)` continues to work for any command, including the ones that now have typed wrappers.
- No `go.mod` changes (still `gorilla/websocket v1.5.3`, minimum Go `1.25.10`).

### Upgrade Notes
- No code changes required when upgrading from `v2.1.x` to `v2.2.0`.
- If you were calling any of the new WebSocket commands through raw `Do(...)`, you can optionally migrate to the typed wrappers.

### Verification Summary
- `go build ./...`, `go vet ./...` pass.
- `go test ./... -race -count=1` passes, including new per-command unit tests and regression tests (`TestWSClient_Connect_NoAutoSupportedFeatures`, `TestWSClient_Do_StillWorks_ForNewCommands`).

## v2.1.2

Security and tooling maintenance release for `v2`.

### Highlights
- Bump minimum Go version, module toolchain, and CI runtime from `1.25.9` to `1.25.10`.
- Resolve reachable `govulncheck` findings from the Go standard library by running/building with patched Go release:
  - GO-2026-4971 (`net`)
  - GO-2026-4918 (`net/http`)

### Compatibility
- No public API changes.
- No import path changes (`github.com/mkelcik/go-ha-client/v2`).
- Existing `v2.x` integrations should continue to work without code changes.

### Upgrade Notes
- Ensure local/dev/CI environments use Go `1.25.10+`.
- If your pipeline pins Go patch versions, update them to `1.25.10` or newer.

### Verification Summary
- `go test ./...` passes.
- `govulncheck ./...` reports no reachable vulnerabilities on Go `1.25.10`.

## v2.1.1

Security and tooling maintenance release for `v2`.

### Highlights
- Bump minimum Go version, module toolchain, and CI runtime from `1.25.8` to `1.25.9`.
- Resolve remaining reachable `govulncheck` findings from the Go standard library by running/building with patched Go release:
  - GO-2026-4946 (`crypto/x509`)
  - GO-2026-4947 (`crypto/x509`)
  - GO-2026-4870 (`crypto/tls`)

### Compatibility
- No public API changes.
- No import path changes (`github.com/mkelcik/go-ha-client/v2`).
- Existing `v2.x` integrations should continue to work without code changes.

### Upgrade Notes
- Ensure local/dev/CI environments use Go `1.25.9+`.
- If your pipeline pins Go patch versions, update them to `1.25.9` or newer.

### Verification Summary
- `go test ./...` passes.
- `govulncheck ./...` reports no reachable vulnerabilities on Go `1.25.9`.

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