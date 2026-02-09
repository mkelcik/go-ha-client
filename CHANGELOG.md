# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and this project follows Semantic Versioning.

## [Unreleased]

### Added
- Debug logging for REST and WebSocket requests/responses.
- Runnable examples for REST and WebSocket flows under `examples/`.
- Helper examples for service command builders and event payload decoding.
- Extra tests for reconnect callbacks and panic-safe callback execution.
- REST helper `CallServiceForEntity` for common entity-targeted service calls.
- WebSocket helper `WaitForStateEquals` for simple state waits without predicate boilerplate.
- WebSocket helper `SubscribeStateChangedMany` for filtering multiple entities in one subscription.
- Generic service command helpers `NewTurnOnCmd`, `NewTurnOffCmd`, and `NewToggleCmd`.
- Typed WebSocket event helper `WSEvent.CallServiceEvent` for `call_service` payload decoding.
- `NewClientWithDefaults(host, token)` shortcut for beginner-friendly client setup.
- New example `examples/ws_wait_for_state_equals` for `WaitForStateEquals`.

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
