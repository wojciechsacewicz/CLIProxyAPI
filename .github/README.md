# wsproxy

Personal fork of [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) — a multi-provider AI API proxy (OpenAI, Claude, Gemini, Codex, and more) with OAuth, load balancing, and TUI support.

Designed to stay cleanly syncable with upstream while layering personal workflow, tooling, and account-routing conventions on top.

## Quick Start

```bash
# Build the wsproxy binary
make build

# Run directly
./wsproxy --config config.yaml

# Or run with the terminal UI
make tui
```

To install `wsproxy` globally into your Go bin directory:

```bash
make install
```

## Docker

Before starting via Docker, make sure `config.yaml` exists (e.g. copy `config.example.yaml` and replace the sample keys):

```bash
docker compose -f docker-compose.wsproxy.yml up -d --build
```

## Syncing with Upstream

The Go module path is intentionally kept aligned with upstream so merges stay clean:

```bash
make sync
```

Or manually:

```bash
git fetch upstream --no-tags
git merge upstream/main
```

## License

[MIT](LICENSE) — based on [router-for-me/CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI).
