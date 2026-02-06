# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and this project follows Semantic Versioning.

## [Unreleased]

### Removed
- Removed `GetDiscoverInfo` and conversation process API support to match REST docs.

### Fixed
- Return `ErrNilHTTPClient` when `NewClient` is called with a nil HTTP client.

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
