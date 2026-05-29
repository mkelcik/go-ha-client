# AGENTS.md

Integration guide for AI coding agents using `github.com/mkelcik/go-ha-client/v2`.

This file is the fast path: minimal context, copy-pasteable snippets, and the
gotchas that bite when you generate code without reading the full README.

## TL;DR

- Go module: `github.com/mkelcik/go-ha-client/v2` (v2 is the stable line).
- Two clients in one package: REST `*Client` and WebSocket `*WSClient`.
- Import alias used throughout examples and docs: `ha`.
- Requires Go `1.25.10+` and a Home Assistant long-lived access token.
- Never invent entity IDs — they must come from the user's HA instance
  (`Developer Tools -> States`).

## Install

```bash
go get github.com/mkelcik/go-ha-client/v2@latest
```

```go
import ha "github.com/mkelcik/go-ha-client/v2"
```

## Decision: REST vs WebSocket

| Use case | Pick | Why |
|---|---|---|
| One-shot read (state, config, history, template render) | REST | Simple request/response, no connection lifecycle. |
| One-shot service call where you don't care about events | REST | `CallServiceForEntity` is the shortest path. |
| Watching `state_changed` or other events live | WebSocket | REST has no push channel. |
| Waiting until an entity reaches a state | WebSocket | Use `WaitForState*` helpers. |
| Long-running daemon with many service calls | WebSocket | One persistent connection, lower overhead. |

When in doubt for an agentic script: start with REST. Switch to WS only when
you need subscriptions or `WaitForState*`.

## Minimal REST client

```go
client, err := ha.NewClientWithDefaults("http://homeassistant.local:8123", "TOKEN")
if err != nil { return err }

if err := client.Ping(ctx); err != nil { return err }

states, err := client.GetStates(ctx)
```

Configurable form:

```go
client, err := ha.NewClient("http://ha.home",
    ha.WithToken("TOKEN"),
    ha.WithTimeout(30*time.Second),
    ha.WithDebug(), // optional: logs requests/responses (tokens redacted)
)
```

## Minimal WebSocket client

```go
ws := client.WS() // reuse the REST client's config (host + token)
if err := ws.Connect(ctx); err != nil { return err }
defer ws.Close()

sub, err := ws.SubscribeStateChanged(ctx, "light.kitchen")
if err != nil { return err }
defer func() {
    // Use a fresh context so cleanup still runs after ctx is canceled.
    cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    _ = sub.Unsubscribe(cleanupCtx)
}()

for {
    select {
    case <-ctx.Done():
        return ctx.Err()
    case ev, ok := <-sub.Events():
        if !ok {
            return nil // subscription closed
        }
        data, ok, err := ev.StateChanged()
        if err != nil { return err }
        if !ok { continue }
        fmt.Println(data.EntityID, data.NewState.State)
    case err := <-sub.Errors():
        return err
    }
}
```

Standalone WS without a REST client:

```go
ws := ha.NewWSClient(ha.ClientConfig{
    Host:  "http://ha.home",
    Token: "TOKEN",
})
```

## API surface cheat-sheet

REST (`*Client`):

- Health/info: `Ping`, `GetConfig`, `GetComponents`, `GetEvents`, `GetServices`.
- State: `GetStates`, `GetStateForEntity`, `CreateState`, `DeleteState`.
- History/logbook: `GetStateChangesHistory`, `GetLogbook`,
  `GetCalendars`, `GetCalendarEvents`.
- Actions: `CallService`, `CallServiceForEntity`, `CallServiceWithResponse`,
  `FireEvent`, `HandleIntent`.
- Misc: `RenderTemplate`, `GetCameraJpeg`, `GetPlainErrorLog`,
  `GetWeatherForecasts`, `TriggerConfigCheck`.

WebSocket (`*WSClient`):

- Lifecycle: `Connect`, `Close`, `IsConnected`, `Ping`.
- Reads: `GetStates`, `GetConfig`, `GetServices`, `GetPanels`,
  `ListEntityRegistryForDisplay`, `ListExposedEntities`.
- Actions: `CallService`, `CallServiceForEntity`, `CallServiceWithResponse`,
  `FireEvent`, `ExposeEntity`, `ValidateConfig`.
- Targets: `ExtractFromTarget`, `GetTriggersForTarget`,
  `GetConditionsForTarget`, `GetServicesForTarget`.
- Subscriptions: `SubscribeEvents`, `SubscribeTrigger`,
  `SubscribeStateChanged`, `SubscribeStateChangedMany`.
- Waits: `WaitForState`, `WaitForStateEquals`, `WaitForStateIn`.
- Escape hatch for any future command: `WSClient.Do(ctx, req, &out)`.

Helpers (package-level):

- `BuildEntityID(domain, objectID)`, `ParseEntityID(entityID)`.
- `NewTurnOnCmd`, `NewTurnOffCmd`, `NewToggleCmd`, `NewTurnLightOnCmd`,
  `NewTurnLightOffCmd`, `NewToggleLightCmd`, `NewServiceDataEntityID`.
- `DecodeEventData[T](event)` — generic typed event decoder.

## Common recipes

### Turn a light on (REST)

```go
_, err := client.CallServiceForEntity(ctx, "light", "turn_on", "light.kitchen", map[string]any{
    "brightness": 200,
})
```

### Turn a light on (WebSocket)

```go
_, err := ws.CallServiceForEntity(ctx, "light", "turn_on", "light.kitchen", map[string]any{
    "brightness": 200,
})
```

### Wait until an entity reaches a state

```go
err := ws.WaitForStateEquals(ctx, "binary_sensor.door", "on")
```

### Subscribe to multiple entities

```go
sub, err := ws.SubscribeStateChangedMany(ctx, "light.kitchen", "light.hall")
```

### Render a Home Assistant template

```go
out, err := client.RenderTemplate(ctx, `{{ states("sun.sun") }}`)
```

### Auto-reconnect WS

```go
ws := client.WS(
    ha.WithAutoReconnect(true),
    ha.WithReconnectBackoff(time.Second, 30*time.Second),
    ha.WithOnReconnect(func() { /* keep short, non-blocking */ }),
)
```

## Error handling

Sentinel errors live in the package root. Match with `errors.Is`:

```go
if errors.Is(err, ha.ErrUnauthorized) { /* bad/expired token */ }
if errors.Is(err, ha.ErrNotFound)     { /* entity or endpoint missing */ }
if errors.Is(err, ha.ErrEmptyEntityID) { /* programmer error */ }
```

Other sentinels: `ErrEmptyCalendarID`, `ErrEmptyTemplate`, `ErrEmptyService`,
`ErrEmptyDomain`, `ErrEmptyEventType`, `ErrEmptyIntentName`.

## Gotchas an agent will hit

1. **Use the real `entity_id`, not the friendly name.** `light.kitchen` is
   valid; `Kitchen Light` is not. If you don't know it, ask the user or call
   `client.GetStates(ctx)` and let them pick. Do not guess.
2. **`v2` import path.** Always `github.com/mkelcik/go-ha-client/v2`. The
   unversioned path is the old `v1` and lacks WS support.
3. **One WS connection per process.** Call `ws.Connect` once and reuse it.
   `WaitForState*` and subscription helpers are safe to use concurrently on
   the same connected client.
4. **Always unsubscribe — with a fresh context.** Subscriptions hold a slot
   in Home Assistant; leaking them across reconnects wastes resources. Don't
   pass the loop's `ctx` to `Unsubscribe` — once that context is canceled,
   cleanup is canceled too. Use `context.Background()` (or a short fresh
   timeout) so the unsubscribe message actually reaches HA.
5. **`sub.Events()` can close.** When it does, the channel returns
   zero-value `WSEvent`s forever. Always use `ev, ok := <-sub.Events()` and
   exit the loop on `!ok`.
6. **Reconnect callbacks are synchronous.** Inside `WithOnReconnect` /
   `WithOnReconnectError`, do not block — spawn a goroutine for any real work.
7. **Event payloads stay as `map[string]interface{}`.** Use
   `WSEvent.StateChanged()`, `WSEvent.CallServiceEvent()`, or
   `ha.DecodeEventData[T]` instead of manual casting.
8. **REST `Host` must include scheme.** `http://ha.home`, not `ha.home`.
9. **Auto-reconnect is opt-in.** Without `WithAutoReconnect(true)` a dropped
   WS stays dropped; the next call returns an error.
10. **Don't add `Co-Authored-By: Claude` or "Generated with Claude Code"** to
    commits or PRs in this repo — see the project's commit history for tone.

## Running locally

```bash
make test        # go test -race ./...
make vet         # go vet ./...
make fmt-check   # fail if gofmt would change files
make ci          # fmt-check + vet + test + lint + staticcheck + gosec + vuln
```

Examples are runnable; edit the constants at the top of each `main.go`:

```bash
go run ./examples/rest_ping
go run ./examples/ws_watch_light_state
```

`examples/helpers_decode_event_data` is the only example that runs offline.

## Where to look next

- [`README.md`](README.md) — human-facing overview and full WS command table.
- [`examples/`](examples/) — every helper and command has a runnable example.
- [`MIGRATION.md`](MIGRATION.md) — for code on `v1`.
- [`CHANGELOG.md`](CHANGELOG.md) — what changed between versions.
- Official HA docs the client tracks:
  - REST: https://developers.home-assistant.io/docs/api/rest
  - WebSocket: https://developers.home-assistant.io/docs/api/websocket
