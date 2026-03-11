# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and this project follows Semantic Versioning.

## [v2.1.0]

### Changed
- Bump minimum Go version, toolchain, and CI from `1.24.13` to `1.25.8`.

### Fixed
- Address standard library vulnerability findings from `govulncheck` by running on Go `1.25.8` (fixes include GO-2026-4601 and GO-2026-4602).

### Changed
- Bump minimum Go version, toolchain, and CI from `1.24.13` to `1.25.8`.

### Fixed
- Address standard library vulnerability findings from `govulncheck` by running on Go `1.25.8` (fixes include GO-2026-4601 and GO-2026-4602).

## [v2.0.0-beta.19]

### Added
- Debug logging for REST and WebSocket requests/responses.
- Runnable examples for REST and WebSocket flows under `examples/`.
- Helper examples for service command builders and event payload decoding.
- Extra tests for reconnect callbacks and panic-safe callback execution.
- REST helper `CallServiceForEntity` for common entity-targeted service calls.
- WebSocket helper `WaitForStateEquals` for simple state waits without predicate boilerplate.
- WebSocket helper `WaitForStateIn` for waiting on multiple target states.
- WebSocket helper `SubscribeStateChangedMany` for filtering multiple entities in one subscription.
- Generic service command helpers `NewTurnOnCmd`, `NewTurnOffCmd`, and `NewToggleCmd`.
- Typed WebSocket event helper `WSEvent.CallServiceEvent` for `call_service` payload decoding.
- `NewClientWithDefaults(host, token)` shortcut for beginner-friendly client setup.
- New example `examples/ws_wait_for_state_equals` for `WaitForStateEquals`.
- New example `examples/ws_watch_selected_entities` for `SubscribeStateChangedMany`.
- New example `examples/ws_watch_call_service_events` for typed `WSEvent.CallServiceEvent`.
- New example `examples/rest_ping_defaults` for `NewClientWithDefaults`.
- New example `examples/ws_wait_for_state_in` for `WaitForStateIn`.

### Removed
- Removed `GetDiscoverInfo` and conversation process API support to match REST docs.

### Changed
- Refactored HTTP request execution into smaller internal helpers.
- Replaced reflection-based query parameter encoding with explicit builders.
- Expanded README and examples documentation, including notes about blocking reconnect callbacks.

### Fixed
- `NewClient` now falls back to `http.DefaultClient` when a nil HTTP client is provided.
- WebSocket connect/close flow now guards against close/connect races.
- WebSocket reconnect logic validates backoff inputs and restores subscriptions more reliably.
- WebSocket subscribe cancellation now deterministically returns `context.Canceled` when cancel races with subscribe result, while still best-effort unsubscribing.

## [v2.0.0-beta.2]

### Changed
- Normalize public API field names to `...ID` and `...URL`.
- Rename `NewToggleLightTCmd` to `NewToggleLightCmd`.
- Bump Go toolchain and CI to `1.24.13`.

### Fixed
- WebSocket subscriptions now use server-provided subscription IDs and unsubscribe on canceled subscribe.
- `ServiceMap.UnmarshalJSON` handles whitespace around `null`.
- WebSocket exported types include GoDoc comments.
- Error message clarifies response body decode failures.

## [v2.0.0-beta.1]

### Added
- WebSocket client, helpers, and examples.
- Project governance docs: `CONTRIBUTING.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md`.
- GitHub issue templates and pull request template.
