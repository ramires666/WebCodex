# WebCodex Gate: Multi-Agent + Web-Admin + SQLite + Docker

WebCodex delivers Codex agent capabilities inside ChatGPT Web with support for **multiple independent worker agents** (Windows / Linux) routed through a single gate server.

Russian version: [README.ru.md](README.ru.md)

---

## Multi-Agent Architecture

```text
                                  INTERNET
                                     │
                                 HTTPS 443
                                     ▼
                            ┌────────────────┐
                            │     Nginx      │
                            │ codex.grom.world
                            └────────┬───────┘
                                     │ :8080 (Docker)
                                     ▼
                       ┌───────────────────────────┐
                       │       webcodex-gate       │
                       │                           │
                       │  /mcp                     │
                       │  /oauth/*                 │
                       │  /agent/stream            │
                       │  /agent/result            │
                       │  /admin                   │
                       │  /healthz                 │
                       │                           │
                       │  SQLite (/data/webcodex.db)
                       │  Runtime Router           │
                       └───┬──────────┬────────┬───┘
                           │          │        │
                     HOME  │    WORK  │   VPS2 │
                           ▼          ▼        ▼
                      Agent #1    Agent #2   Agent #3
                      (Windows)   (Windows)  (Linux)
```

- **Single Domain**: e.g. `https://codex.grom.world`.
- **Multi-Agent Routing**: Independent streams and queues for `home`, `work`, `vps` without cross-agent contamination.
- **SQLite Persistence**: Embedded database using CGO-free `modernc.org/sqlite` with WAL mode and foreign keys.
- **Security**: Only SHA-256 hashes of agent tokens and OAuth secrets are stored.
- **Web Admin**: Built-in dark-themed web panel at `/admin` protected by HTTP Basic Auth and CSRF tokens.

---

## Quickstart with Docker and Nginx

### 1. Configure Environment

```bash
cp .env.example .env
```

Edit `.env`:

```env
MCP_DOMAIN=codex.grom.world
WEBCODEX_ADMIN_USER=admin
WEBCODEX_ADMIN_PASSWORD=YOUR_STRONG_RANDOM_PASSWORD
```

### 2. Start the Gate Container

```bash
docker compose up -d --build
```

### 3. Nginx Reverse Proxy Setup

Use `deploy/nginx-codex.grom.world.conf` to proxy traffic to `127.0.0.1:8080` with buffering disabled for streaming.

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;

    proxy_buffering off;
    proxy_cache off;
    chunked_transfer_encoding on;

    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
```

---

## Running Worker Agent (Windows PowerShell)

```powershell
$env:WEBCODEX_GATE_URL = "https://codex.grom.world"
$env:WEBCODEX_AGENT_TOKEN = "wc_agent_TOKEN_FROM_ADMIN"

.\webcodex-agent.exe
```

---

## ChatGPT OAuth Configuration

| Parameter | Value |
| --- | --- |
| Server URL | `https://codex.grom.world/mcp` |
| Authorization URL | `https://codex.grom.world/oauth/authorize` |
| Token URL | `https://codex.grom.world/oauth/token` |
| Client ID | `wc_client_...` (from /admin for selected agent) |
| Client Secret | `wc_oauth_...` (from /admin for selected agent) |
| Auth type | OAuth 2.0 (Authorization Code + PKCE) |
