# Contributing

Thanks for contributing to `go-ha-client`!

## Development setup

Requirements:
- Go `1.25.12+`
- Make

Clone and run checks:

```bash
make tools
make ci
```

## Workflow

1. Create a branch from `main`.
2. Make focused changes.
3. Run `make ci`.
4. Open a pull request.

## Pull request checklist

- [ ] Tests were added or updated when behavior changed.
- [ ] `make ci` passes locally.
- [ ] Public API changes are documented in `README.md`.
- [ ] Breaking changes are clearly marked.

## Commit messages

Use clear commit messages. Conventional Commit style is preferred, for example:

- `feat: add websocket call_service helper`
- `fix: return fallback error for invalid 400 body`
- `docs: add websocket subscription example`

## Coding notes

- Keep exported APIs backward compatible within `v2.x`.
- Prefer sentinel errors and `errors.Is` checks for public error behavior.
- Keep examples runnable and concise.
