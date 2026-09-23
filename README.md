# WebCodex Gate: Multi-Agent + Web-Admin + SQLite + Docker

WebCodex delivers Codex agent capabilities inside ChatGPT Web with support for **multiple independent worker agents** (Windows / Linux) routed through a single gate server.

Russian version: [README.ru.md](README.ru.md) | User Guide: [USER_GUIDE.ru.md](USER_GUIDE.ru.md)

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

## 🚀 Quickstart: Installation, Build & Setup

Everything is designed to be as automated as possible.

### Step 1. Install Go (Golang)

You need Go (version 1.22+) to compile the agent or gate.

#### Windows (one command via winget)
Open **PowerShell** and run:
```powershell
winget install GoLang.Go
```
*(Or via Chocolatey: `choco install golang`, or download installer from [go.dev/dl](https://go.dev/dl/)).*

> 💡 **Tip**: Reopen your terminal after installation so that `PATH` takes effect. Verify with `go version`.

#### Linux (Ubuntu / Debian)
```bash
sudo apt update && sudo apt install -y golang
```

#### macOS
```bash
brew install go
```

---

### Step 2. Install Codex CLI

The agent delegates execution safely to the official OpenAI Codex CLI on your machine:
```bash
npm install -g @openai/codex
```
Verify with:
```bash
codex --version
```

---

### Step 3. Compile the Programs (One-Click Build)

Ready-to-use automated build scripts are included:

#### Windows:
Double-click **`build.bat`** (or execute in terminal):
```cmd
build.bat
```
This script automatically:
- Detects Go (even before PATH refresh).
- Compiles `webcodex-agent.exe` (client worker).
- Compiles `bin\webcodex-gate.exe` (gate server).

Or build manually via PowerShell:
```powershell
go build -ldflags="-s -w" -o webcodex-agent.exe ./cmd/agent
```

#### Linux / macOS:
```bash
chmod +x build.sh
./build.sh
```

---

### Step 4. Run the WebCodex Agent

The easiest automated way is using the Web Admin panel:

1. Open the Web Admin in your browser: **`https://codex.grom.world/admin`**.
2. Under **"Add New Agent"**:
   - Provide an **Agent ID** (e.g. `home`, `work` — lowercase letters, digits, dashes).
   - Enter a human-readable title.
   - Click **"Create Agent"**.
3. On the credentials page, click the green button:
   👉 **`📥 Download start-agent-<id>.bat`**
4. Place the downloaded `.bat` file in the same folder as `webcodex-agent.exe`.
5. **Double-click the `.bat` file**.

> 🎉 The worker agent will connect to the gate, and the admin panel status will turn to **`🟢 ONLINE`**.

#### Manual Run (PowerShell):
```powershell
$env:WEBCODEX_GATE_URL = "https://codex.grom.world"
$env:WEBCODEX_AGENT_TOKEN = "wc_agent_TOKEN_FROM_ADMIN"

.\webcodex-agent.exe
```

---

### Step 5. Connect to ChatGPT Web / Plugins / Actions
### Step 5. Connect to ChatGPT Web (Connected Apps / MCP)

Connect ChatGPT once per agent:
> ⚠️ **Note**: WebCodex implements the **MCP (Model Context Protocol)** standard over HTTP, **not** a REST OpenAPI schema!
> **Do NOT paste the URL into the "Schema" box of Custom GPT Actions.** The Schema box expects an OpenAPI YAML/JSON document, and pasting a plain URL will cause `Could not find a valid URL in 'servers'`.
> With MCP, no manual schema is required — ChatGPT dynamically queries all available tools directly from the server.

#### Method 1: Via Connected Apps / Chat MCP Tools (Recommended)
#### Connecting via Connected Apps / Developer Mode:
1. Open [chatgpt.com](https://chatgpt.com).
2. Go to **Settings ➔ Connected Apps** (or Developer Mode / Add MCP server in chat).
3. Click **Connect new app / Add MCP Server**.
4. Paste the credentials from the admin panel (or from the downloaded `chatgpt-oauth-<id>.txt`):
2. Click your user profile in the bottom-left corner ➔ **Settings**.
3. Select **Connected Apps** (or **Developer**).
4. Click **Connect new app** or **Add MCP Server**.
5. Fill in the parameters from your Web Admin (`/admin`):

| Parameter | Value |
| :--- | :--- |
| **Server URL** | `https://codex.grom.world/mcp` |
| **Authorization URL** | `https://codex.grom.world/oauth/authorize` |
| **Token URL** | `https://codex.grom.world/oauth/token` |
| **Client ID** | `wc_client_...` (from /admin for selected agent) |
| **Client Secret** | `wc_oauth_...` (from /admin for selected agent) |
| **Auth type** | `OAuth 2.0` (Authorization Code + PKCE) |

5. Click **Connect**. ChatGPT will authenticate with your gate server and activate the connection.
6. Now you can instruct ChatGPT in chat to inspect code, run terminal commands, and perform development work directly on your machine through your local Codex instance!
6. Click **Connect**. Authorize the gate server, and ChatGPT will immediately activate the connection.
7. Now in any ChatGPT conversation, you can ask ChatGPT to inspect projects, run scripts, and execute commands on your computer through your local worker agent!

#### Method 2: Via Custom GPT (Actions)
1. Go to **Explore GPTs** ➔ **+ Create**.
2. Switch to the **Configure** tab.
3. Under **Actions**, click **Create new action**.
4. Set the Schema URL to your gate server MCP endpoint (`https://codex.grom.world/mcp`).
5. In **Authentication**, select **OAuth** with Authorization URL, Token URL, Client ID, and Client Secret from your admin panel.
6. Save and start chatting!

---

## Server Deployment: Docker Compose & Nginx

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

Use `deploy/nginx-codex.grom.world.conf` to proxy traffic to `127.0.0.1:8080` with buffering disabled for streaming:

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

## Local Development & Tests

```powershell
# Run tests
go test -v ./cmd/gate/...
go test -v ./cmd/agent/...

# Manual compilation
go build -o bin/webcodex-gate.exe ./cmd/gate
go build -o webcodex-agent.exe ./cmd/agent
```
