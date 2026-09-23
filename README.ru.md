# WebCodex Gate: Multi-Agent + Web-Admin + SQLite + Docker

WebCodex предоставляет агентские возможности Codex внутри ChatGPT Web с поддержкой **множества независимых рабочих агентов** (Windows / Linux) через единый шлюз.

**Лимиты Codex при этом не расходуются: работа идет через ChatGPT Web, поэтому используется только лимит текущего чата, а опыт остается близким к Codex.**

English version: [README.md](README.md) | Полное руководство: [USER_GUIDE.ru.md](USER_GUIDE.ru.md)

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

## 🚀 Быстрая установка и запуск агента (от 0 до работы)

Все шаги максимально автоматизированы. Выполните их по порядку на вашем рабочем компьютере (Windows / Linux / macOS).

### Шаг 1. Установка Go (Golang)

Для компиляции агента из исходников необходим компилятор Go (версии 1.22+).

#### Windows (автоматически в 1 команду)
Откройте **PowerShell** от имени администратора или обычного пользователя и выполните:
```powershell
winget install GoLang.Go
```
*(Или через Chocolatey: `choco install golang`, либо скачайте MSI-установщик с официального сайта [go.dev/dl](https://go.dev/dl/)).*

> 💡 **Важно**: После завершения установки закройте и снова откройте терминал, чтобы применились системные переменные PATH. Проверьте установку командой:
> ```powershell
> go version
> ```

#### Linux (Ubuntu / Debian)
```bash
sudo apt update && sudo apt install -y golang
```

#### macOS
```bash
brew install go
```

---

### Шаг 2. Установка движка Codex CLI

Агент WebCodex использует официальный CLI OpenAI Codex для безопасного исполнения команд и редактирования кода на вашем компьютере:

```bash
npm install -g @openai/codex
```

Проверьте доступность:
```bash
codex --version
```

---

### Шаг 3. Сборка программы (компиляция в 1 клик)

В репозиторий включены автоматические скрипты сборки, которые сами найдут установленный Go и скомпилируют все бинарники с оптимизацией размера:

#### На Windows:
Дважды кликните по файлу **`build.bat`** в папке проекта (или запустите в консоли):
```cmd
build.bat
```
Скрипт автоматически:
- Найдет Go (даже если переменная PATH еще не успела обновиться).
- Соберет `webcodex-agent.exe` (клиент для вашего ПК).
- Соберет `bin\webcodex-gate.exe` (шлюз для сервера).

#### Ручная сборка на Windows (PowerShell):
```powershell
go build -ldflags="-s -w" -o webcodex-agent.exe ./cmd/agent
```

#### На Linux / macOS:
```bash
chmod +x build.sh
./build.sh
```
*(или вручную: `go build -ldflags="-s -w" -o webcodex-agent ./cmd/agent`)*

---

### Шаг 4. Запуск агента WebCodex

Самый простой и автоматический способ — использовать веб-панель шлюза:

1. Откройте веб-админку вашего шлюза в браузере: **`https://codex.grom.world/admin`** *(или ваш домен)*.
2. В блоке **«Добавить нового агента»**:
   - Укажите **ID агента** (например, `home`, `work`, `laptop` — латинские буквы без пробелов).
   - Введите понятное название (например, `Домашний ПК`).
   - Нажмите кнопку **«Создать агента»**.
3. На открывшейся странице нажмите большую зеленую кнопку:
   👉 **`📥 Скачать start-agent-<id>.bat`**
4. Положите скачанный `.bat` файл в одну папку с `webcodex-agent.exe`.
5. **Запустите `.bat` файл двойным кликом**.

> 🎉 **Агент запущен!** В окне консоли появится сообщение о подключении к шлюзу, а в админке статус вашего агента сразу перейдет в **`🟢 ONLINE`**.

#### Запуск вручную без скачивания .bat:
В PowerShell:
```powershell
$env:WEBCODEX_GATE_URL = "https://codex.grom.world"
$env:WEBCODEX_AGENT_TOKEN = "wc_agent_ВАШ_ТОКЕН_ИЗ_АДМИНКИ"

.\webcodex-agent.exe
```

---

### Шаг 5. Добавление агента в ChatGPT чат

> ⚠️ **Важно**: WebCodex работает по протоколу **MCP (Model Context Protocol)**, а **НЕ** через REST/OpenAPI!
> **НЕ вставляйте URL в поле «Schema» при создании Custom GPT Actions** — поле Schema ожидает OpenAPI YAML/JSON, и выдаст ошибку `Could not find a valid URL in 'servers'`.
> WebCodex подключается без каких-либо схем — ChatGPT сам получает список инструментов на лету.

#### Подключение через Connected Apps (Связанные приложения / MCP):
1. Откройте [chatgpt.com](https://chatgpt.com).
2. Нажмите на ваш профиль в левом нижнем углу ➔ **Settings (Настройки)**.
3. Перейдите во вкладку **Connected Apps (Связанные приложения)** *(в некоторых интерфейсах: **Developer / Для разработчиков**)*.
4. Нажмите **Connect new app (Подключить приложение)** или **Add MCP Server**.
5. Заполните 5 полей из панели управления `/admin` (или из файла `chatgpt-oauth-<id>.txt`):

| Поле в форме ChatGPT | Значение для вставки |
| :--- | :--- |
| **Server URL** | `https://codex.grom.world/mcp` |
| **Authorization URL** | `https://codex.grom.world/oauth/authorize` |
| **Token URL** | `https://codex.grom.world/oauth/token` |
| **Client ID** | `wc_client_...` *(из карточки агента в /admin)* |
| **Client Secret** | `wc_oauth_...` *(из карточки агента в /admin)* |
| **Auth Type** | `OAuth 2.0` *(Authorization Code + PKCE)* |

6. Нажмите **Connect (Подключить)**. ChatGPT перенаправит вас на страницу подтверждения авторизации шлюза и моментально вернется обратно.


#### 💬 Как пользоваться в чате:
После подключения в чате ChatGPT появляется доступ к инструментам вашего компьютера:
- Попросите ChatGPT: *"Посмотри структуру проекта в папке C:\projects\my-app и найди ошибки в тестах"*.
- ChatGPT вызовет ваш локальный WebCodex Agent, тот выполнит команду через Codex CLI на вашей машине и вернет результат в чат.
- Вы увидите карточки вызовов инструментов прямо в интерфейсе!

---

## 🛠️ Развертывание шлюза (Gate Server) в Docker за Nginx

Если вы настраиваете свой собственный сервер шлюза:

### 1. Настройка переменных окружения

```bash
cp .env.example .env
```

Отредактируйте `.env`:
```env
MCP_DOMAIN=codex.grom.world
WEBCODEX_ADMIN_USER=admin
WEBCODEX_ADMIN_PASSWORD=УКАЖИТЕ_СЛОЖНЫЙ_СЛУЧАЙНЫЙ_ПАРОЛЬ
```

### 2. Запуск контейнера шлюза

```bash
docker compose up -d --build
```

Шлюз поднимется и будет слушать порт `127.0.0.1:8080` на хосте.

### 3. Конфигурация Nginx

Скопируйте `deploy/nginx-codex.grom.world.conf` в `/etc/nginx/sites-available/codex.grom.world`:

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

        # Отключение буферизации для потоков NDJSON (/agent/stream) и SSE (/mcp)
        proxy_buffering off;
        proxy_cache off;
        chunked_transfer_encoding on;

        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
}
```

Активируйте и перезагрузите Nginx:
```bash
ln -s /etc/nginx/sites-available/codex.grom.world /etc/nginx/sites-enabled/
nginx -t && systemctl reload nginx
```

---

## 🔐 Возможности веб-панели управления (/admin)

Панель доступна по адресу `https://codex.grom.world/admin`:
- **Создание агентов** за несколько секунд.
- **Автоматическая генерация файлов запуска** (`start-agent-<id>.bat`, `start-agent-<id>.ps1`, `agent.env`, `chatgpt-oauth.txt`) для скачивания в 1 клик.
- **Мониторинг в реальном времени**: онлайн/офлайн статус, время последнего отклика.
- **Управление политиками доступа**: белый список инструментов (`AllowedTools`) и черный список (`DeniedTools`) индивидуально для каждого агента.
- **Безопасная ротация ключей**: перевыпуск токенов агента и OAuth-секретов в 1 клик.

---

## 🧪 Разработка и тесты

```powershell
# Запуск полного набора юнит-тестов шлюза и роутера
go test -v ./cmd/gate/...

# Запуск тестов агента
go test -v ./cmd/agent/...
```
