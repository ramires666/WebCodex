# WebCodex Gate: Multi-Agent + Web-Admin + SQLite + Docker

WebCodex предоставляет агентские возможности Codex внутри ChatGPT Web с поддержкой **множества независимых рабочих агентов** (Windows / Linux) через единый шлюз.

**Лимиты Codex при этом не расходуются: работа идет через ChatGPT Web, поэтому используется только лимит текущего чата, а опыт остается близким к Codex.**

English version: [README.md](README.md)

---

## Архитектура Multi-Agent

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

1. **Единый публичный домен**: например `https://codex.grom.world`.
2. **Многоагентность**: одновременное подключение любого количества рабочих станций (Home PC, Work PC, VPS).
3. **Изоляция сессий**: запросы ChatGPT для агента `home` попадают строго на машину `home`.
4. **Хранилище SQLite**: база `/data/webcodex.db` (на базе `modernc.org/sqlite` без CGO) сохраняет агентов, настройки инструментов и выданные токены доступа.
5. **Безопасность**: в БД хранятся исключительно SHA-256 хеши токенов и секретов.
6. **Веб-панель управления**: доступна по адресу `https://codex.grom.world/admin` (защищена HTTP Basic Auth и CSRF).

---

## Быстрый старт в Docker за Nginx

### 1. Настройка переменных окружения

Создайте `.env` из примера:

```bash
cp .env.example .env
```

Отредактируйте `.env`:

```env
MCP_DOMAIN=codex.grom.world
WEBCODEX_ADMIN_USER=admin
WEBCODEX_ADMIN_PASSWORD=УКАЖИТЕ_СЛОЖНЫЙ_СЛУЧАЙНЫЙ_ПАРОЛЬ
```

### 2. Запуск контейнера

```bash
docker compose up -d --build
```

Шлюз поднимется и будет слушать порт `127.0.0.1:8080` на хосте.

### 3. Настройка Nginx на сервере

Скопируйте конфигурацию из `deploy/nginx-codex.grom.world.conf` в `/etc/nginx/sites-available/codex.grom.world`:

```nginx
server {
    listen 80;
    listen [::]:80;
    server_name codex.grom.world;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name codex.grom.world;

    # SSL сертификаты (Certbot / Let's Encrypt)
    ssl_certificate /etc/letsencrypt/live/codex.grom.world/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/codex.grom.world/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;

        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Важно: отключение буферизации для NDJSON (/agent/stream) и SSE (/mcp)
        proxy_buffering off;
        proxy_cache off;
        chunked_transfer_encoding on;

        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
}
```

Активируйте сайт и перезагрузите Nginx:

```bash
ln -s /etc/nginx/sites-available/codex.grom.world /etc/nginx/sites-enabled/
nginx -t && systemctl reload nginx
```

---

## Использование панели управления (/admin)

1. Откройте в браузере: `https://codex.grom.world/admin`.
2. Введите имя пользователя и пароль из `.env`.
3. В панели можно:
   - Создать нового агента (например `home`, `work`).
   - Получить сгенерированные учетные данные:
     - **Windows Agent Token** (для `webcodex-agent.exe`).
     - **OAuth Client ID** и **OAuth Client Secret** (для ChatGPT).
   - Следить за статусом online / offline и временем активности.
   - Включать/отключать агентов.
   - Ротировать токены агента и секреты OAuth в один клик.
   - Настраивать разрешенные (`AllowedTools`) и запрещенные (`DeniedTools`) инструменты per-agent.

---

## Запуск агента на Windows

На рабочей машине скачайте `webcodex-agent.exe` и запустите в PowerShell:

```powershell
$env:WEBCODEX_GATE_URL = "https://codex.grom.world"
$env:WEBCODEX_AGENT_TOKEN = "wc_agent_ВАШ_ТОКЕН_ИЗ_АДМИНКИ"

.\webcodex-agent.exe
```

После запуска в панели `/admin` статус агента перейдет в **Online**.

---

## Подключение в ChatGPT

В ChatGPT перейдите в **Settings -> Connected apps / Custom Actions** и создайте новое подключение:

| Параметр | Значение |
| --- | --- |
| Server URL | `https://codex.grom.world/mcp` |
| Authorization URL | `https://codex.grom.world/oauth/authorize` |
| Token URL | `https://codex.grom.world/oauth/token` |
| Client ID | `wc_client_...` (из админки для выбранного агента) |
| Client Secret | `wc_oauth_...` (из админки для выбранного агента) |
| Auth type | OAuth 2.0 (Authorization Code + PKCE) |

---

## Локальная разработка и тестирование

```powershell
# Запуск тестов
go test -v ./cmd/gate/...

# Сборка бинарника
go build -o bin/webcodex-gate.exe ./cmd/gate
```
