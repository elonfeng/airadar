# airadar

Detect trending AI products and news from multiple sources.

## Features

- **7 data sources**: Hacker News, GitHub, Reddit, ArXiv, Twitter/X, YouTube, RSS feeds
- **Trend detection**: Cross-source correlation, velocity scoring, topic clustering
- **Smart filtering**: AI keyword matching with customizable rules
- **Alerts**: Slack, Discord, generic webhook notifications
- **Dual interface**: CLI tool + HTTP API
- **Lightweight**: SQLite storage, single binary, zero external dependencies

## Install

```bash
go install github.com/elonfeng/airadar/cmd/airadar@latest
```

## Quick Start

```bash
# collect data from all enabled sources
airadar collect

# collect from specific sources
airadar collect --source=hn,github,rss

# view trending topics
airadar trends

# start daemon (scheduler + HTTP API)
airadar run --port=8080
```

## Configuration

Copy `config.example.yaml` to `config.yaml` and customize:

```bash
cp config.example.yaml config.yaml
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `GITHUB_TOKEN` | GitHub API token (optional, higher rate limits) |
| `REDDIT_CLIENT_ID` | Reddit OAuth2 client ID |
| `REDDIT_CLIENT_SECRET` | Reddit OAuth2 client secret |
| `YOUTUBE_API_KEY` | YouTube Data API v3 key |
| `SLACK_WEBHOOK_URL` | Slack incoming webhook URL |
| `DISCORD_WEBHOOK_URL` | Discord webhook URL |
| `OPENAI_API_KEY` | OpenAI API key (enables LLM evaluation) |
| `ANTHROPIC_API_KEY` | Anthropic API key (enables LLM evaluation) |
| `AIRADAR_DB_PATH` | SQLite database path (default: ./airadar.db) |

### Sources

| Source | Auth Required | Default |
|--------|--------------|---------|
| Hacker News | No | Enabled |
| GitHub Trending | Optional token | Enabled |
| Reddit | OAuth2 credentials | Disabled |
| ArXiv | No | Enabled |
| Twitter/X | No (Nitter RSS) | Disabled |
| YouTube | API key | Disabled |
| RSS Feeds | No | Enabled |

## HTTP API

```bash
# start server
airadar serve --port=8080

# get trending topics
curl http://localhost:8080/api/v1/trends

# get collected items
curl http://localhost:8080/api/v1/items?source=hackernews

# trigger collection
curl -X POST http://localhost:8080/api/v1/collect

# list sources
curl http://localhost:8080/api/v1/sources

# health check
curl http://localhost:8080/health
```

## Trend Detection

The trend engine uses three weighted scoring strategies:

1. **Cross-Source Score (50%)** — Same topic appearing on multiple platforms indicates real virality. Uses Jaccard similarity for title matching and Union-Find clustering.

2. **Velocity Score (30%)** — How fast an item's score is growing. Tracks score snapshots over time and calculates growth rate.

3. **Absolute Score (20%)** — Raw score normalized by source type. HN 500 points ≠ Reddit 500 upvotes.

Topics scoring above the threshold (default: 30) trigger alerts.

### LLM Evaluation (Optional)

When enabled, all collected items are sent to an LLM in **one batch API call** per detection cycle. The LLM scores each item 0-10 for AI relevance and importance, filtering out noise before trend scoring. Only items above the threshold (default: 6) are kept.

- **Cost**: ~$0.01-0.05 per cycle with `gpt-4o-mini`, depending on item count
- **Frequency**: Once per trend detection interval (default: every 30 minutes)
- **Providers**: OpenAI (`gpt-4o-mini`, `gpt-4o`) or Anthropic (`claude-sonnet-4-20250514`)
- **Fallback**: If LLM call fails, falls back to pure algorithmic detection

Enable via environment variable:

```bash
export OPENAI_API_KEY="sk-..."   # auto-enables LLM with gpt-4o-mini
# or
export ANTHROPIC_API_KEY="sk-..."  # auto-enables LLM with Claude
```

## Deploy

### Docker

```bash
docker build -t airadar .
docker run -p 8080:8080 -v airadar-data:/data airadar
```

With alert configuration:

```bash
docker run -p 8080:8080 -v airadar-data:/data \
  -e SLACK_WEBHOOK_URL="https://hooks.slack.com/services/..." \
  -e GITHUB_TOKEN="ghp_..." \
  airadar
```

## Architecture

```
Sources (7)          Trend Engine           Alerts
┌──────────┐     ┌────────────────┐     ┌──────────┐
│ HN       │────▶│ Topic Cluster  │────▶│ Slack    │
│ GitHub   │     │ Cross-Source   │     │ Discord  │
│ Reddit   │     │ Velocity       │     │ Webhook  │
│ ArXiv    │     │ Absolute Score │     └──────────┘
│ Twitter  │     └───────┬────────┘
│ YouTube  │             │
│ RSS      │             ▼
└──────────┘     ┌──────────────┐
       │         │   SQLite DB  │
       └────────▶│   (items,    │
                 │   snapshots, │
                 │   trends)    │
                 └──────────────┘
```

## License

MIT
