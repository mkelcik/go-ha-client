# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and this project follows Semantic Versioning.

## [v2.2.0] - 2026-04-20

### Added
- Typed WebSocket helpers covering every command documented in the official WebSocket API:
  - `WSClient.DeclareSupportedFeatures` (opt-in `supported_features`; Connect still never sends it automatically).
  - `WSClient.GetPanels` (`get_panels`).
  - `WSClient.ValidateConfig` (`validate_config`).
  - `WSClient.ExtractFromTarget` (`extract_from_target`).
  - `WSClient.GetTriggersForTarget`, `GetConditionsForTarget`, `GetServicesForTarget` (`get_*_for_target`).
  - `WSClient.ListEntityRegistryForDisplay` (`config/entity_registry/list_for_display`).
  - `WSClient.ListExposedEntities`, `WSClient.ExposeEntity` (`homeassistant/expose_entity[/list]`).
- New public types in `types.go`: `Panel`, `Panels`, `TargetSelector`, `ValidateConfigRequest`, `ValidateConfigSectionResult`, `ValidateConfigResult`, `ExtractFromTargetResult`, `TriggerInfo`, `ConditionInfo`, `ServiceTargetInfo`, `DisplayEntity`, `DisplayEntityRegistry`, `ExposedEntitiesResult`, `ExposeEntityRequest`.
- New sentinels `ErrEmptyTarget` and `ErrEmptyAssistants` for empty target selectors and `ExposeEntity` calls without target assistants.
- Runnable examples: `examples/ws_panels`, `examples/ws_validate_config`, `examples/ws_entity_registry`, `examples/ws_expose_entity`, `examples/ws_target_helpers`, `examples/ws_supported_features`.
- README section listing the full WebSocket command → method mapping.

### Notes
- Purely additive release; all `v2.1.x` integrations keep working without changes.
- `Connect()` handshake sequence is unchanged: `supported_features` remains opt-in via `DeclareSupportedFeatures`.

## [v2.1.2]

### Changed
- Bump minimum Go version, toolchain, and CI from `1.25.9` to `1.25.10`.

### Fixed
- Address standard library vulnerability findings from `govulncheck` by running on Go `1.25.10` (fixes include GO-2026-4971 (`net`) and GO-2026-4918 (`net/http`)).

## [v2.1.1]

### Changed
- Bump minimum Go version, toolchain, and CI from `1.25.8` to `1.25.9`.

### Fixed
- Address remaining standard library vulnerability findings from `govulncheck` by running on Go `1.25.9` (fixes include GO-2026-4946, GO-2026-4947, and GO-2026-4870).

## [v2.1.0]

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