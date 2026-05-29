# Migration Guide

## v2.1.x -> v2.2.0

No migration required. `v2.2.0` is a purely additive release:
- Adds typed WebSocket helpers for every command in the official WebSocket API docs (`GetPanels`, `ValidateConfig`, `ExtractFromTarget`, `GetTriggersForTarget`, `GetConditionsForTarget`, `GetServicesForTarget`, `ListEntityRegistryForDisplay`, `ListExposedEntities`, `ExposeEntity`, `DeclareSupportedFeatures`).
- Adds new public types and new sentinel errors `ErrEmptyTarget` and `ErrEmptyAssistants`.
- Does not change any existing method signatures, types, or the `Connect()` handshake (`supported_features` is opt-in, never sent automatically).

Upgrade by bumping the dependency version and rerunning tests.

## v2.0.0-beta.x -> v2.0.0

- No module path change (`github.com/mkelcik/go-ha-client/v2` stays the same).
- No intentional breaking public API changes are introduced in `v2.0.0` relative to documented v2 beta APIs.
- Upgrade by bumping the dependency version and rerunning tests in your integration environment.

## v1 -> v2

1) Update module path
- Old: `github.com/mkelcik/go-ha-client`
- New: `github.com/mkelcik/go-ha-client/v2`

2) Client initialization
```go
client, err := ha.NewClient(
	"http://my-ha.home",
	ha.WithToken("token"),
	ha.WithHTTPClient(&http.Client{Timeout: 30 * time.Second}),
)
```

3) API renames and removals
- Exported `...Id` fields are now `...ID`.
- `NewToggleLightTCmd` was renamed to `NewToggleLightCmd`.
- `GetDiscoverInfo` and conversation process REST helpers were removed to match the REST docs.
