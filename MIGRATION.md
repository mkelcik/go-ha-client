# Migration Guide

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
