# WebCodex Gate: multi-agent + web-admin + SQLite + Docker

## 0. Что именно мы хотим получить

Исходный WebCodex состоит из двух программ:

- `webcodex-gate` — публичный HTTP/MCP сервер;
- `webcodex-agent` — программа на рабочем компьютере, которая устанавливает исходящее соединение с gate и выполняет MCP-вызовы локально.

В текущем upstream `webcodex-gate` рассчитан фактически на одного агента: `server` содержит один `publicToken`, один `agentToken`, одну `queue`, один `pending` и один `stream`. При подключении нового agent stream предыдущий stream отменяется.

Текущие публичные маршруты включают `/mcp`, OAuth endpoints, `/agent/stream`, `/agent/result` и `/healthz`.

Наша задача — переделать gate в такую систему:

```text
                     ┌─────────────────────────────┐
                     │      webcodex-gate          │
                     │                             │
ChatGPT #1 ─────────►│ OAuth/MCP ──► Agent HOME   ├────► Windows HOME
                     │                             │
ChatGPT #2 ─────────►│ OAuth/MCP ──► Agent WORK   ├────► Windows WORK
                     │                             │
ChatGPT #3 ─────────►│ OAuth/MCP ──► Agent VPS2   ├────► Windows/VPS #3
                     │                             │
Admin Browser ──────►│ /admin                     │
                     │      │                      │
                     │      ▼                      │
                     │   SQLite DB                 │
                     └─────────────────────────────┘
```

В результате мы хотим:

1. Один публичный домен, например:

```text
https://mcp.example.com
```

2. Один Docker-контейнер `webcodex-gate`.

3. Один SQLite-файл:

```text
/data/webcodex.db
```

4. Неограниченное разумное количество агентов.

5. У каждого агента собственные:

```text
agent token
OAuth client ID
OAuth client secret
MCP access tokens
allowed tools
denied tools
```

6. Админка:

```text
https://mcp.example.com/admin
```

7. В админке можно:

```text
создать агента
посмотреть online/offline
отключить агента
включить агента
перевыпустить agent token
перевыпустить OAuth secret
отозвать MCP access tokens
настроить allowed/denied tools
удалить агента
```

8. Существующий Windows `webcodex-agent.exe` по возможности **не менять**.

Это возможно, потому что текущий агент уже использует Bearer-токен для `/agent/stream` и `/agent/result`; gate может определить `agent_id` по самому токену.

---

# 1. Не начинай с Docker

Очень важное правило для junior-разработчика:

```text
Сначала код работает локально.
Потом тесты.
Потом бинарник.
И только потом Docker.
```

Не пытайся одновременно:

```text
переделывать архитектуру
+
чинить SQLite
+
чинить OAuth
+
чинить Docker
+
чинить Caddy
```

Иначе при ошибке будет непонятно, где именно она находится.

Работать будем этапами.

---

# 2. Сделай отдельную ветку

Клонируем репозиторий:

```bash
git clone https://github.com/dm-vev/WebCodex.git
cd WebCodex
```

Проверяем:

```bash
git status
```

Создаём ветку:

```bash
git checkout -b feature/multi-agent-gate
```

Проверяем:

```bash
git branch
```

Должна быть звёздочка:

```text
* feature/multi-agent-gate
  main
```

До любых изменений проверь исходный проект:

```bash
go test ./...
```

Затем:

```bash
go vet ./...
```

И:

```bash
go build -o bin/webcodex-gate ./cmd/gate
```

Именно такая команда сборки gate указана в текущем README проекта. Модуль сейчас объявляет `go 1.23`.

Если исходный проект уже не собирается — **не начинай рефакторинг**.

Сначала запиши ошибку и разберись с baseline.

---

# 3. Посмотри на текущую структуру

Основные файлы gate сейчас находятся здесь:

```text
cmd/gate/
├── agent.go
├── http_helpers.go
├── main.go
├── mcp.go
├── mcp_test.go
├── oauth.go
├── server.go
├── server_test.go
├── tool_card_resource.go
├── tool_cards.go
├── tool_policy.go
├── templates/
│   └── tool-card.html
└── ...
```

Эта структура соответствует текущей ветке `main`.

Особенно внимательно прочитай:

```text
server.go
agent.go
mcp.go
oauth.go
http_helpers.go
tool_policy.go
```

Не просто копируй следующий код. Сначала пойми текущий поток.

---

# 4. Пойми текущий поток запроса

Сейчас MCP-клиент вызывает:

```text
POST /mcp
Authorization: Bearer PUBLIC_TOKEN
```

В `handleMCP()` происходит примерно:

```text
проверить PUBLIC_TOKEN
        ↓
прочитать JSON-RPC
        ↓
если initialize → ответить локально
        ↓
если tools/call → проверить tool policy
        ↓
callAgent()
        ↓
положить запрос в общую queue
        ↓
agent забирает запрос через /agent/stream
        ↓
agent отправляет результат в /agent/result
        ↓
pending[id] получает ответ
        ↓
ответ возвращается MCP клиенту
```

Текущий `handleMCP()` действительно проверяет один `s.publicToken`, после чего использует общие `s.enqueue()` и `s.callAgent()`.

А `/agent/stream` сейчас проверяет один:

```go
s.agentToken
```

и читает:

```go
s.queue
```

для всего сервера.

Это и нужно изменить.

---

# 5. Новая концепция

Вместо:

```go
server
 ├── publicToken
 ├── agentToken
 ├── queue
 ├── pending
 └── stream
```

должно стать:

```text
server
│
├── database
│
├── runtimes
│   │
│   ├── home
│   │   ├── queue
│   │   ├── pending
│   │   └── stream
│   │
│   ├── work
│   │   ├── queue
│   │   ├── pending
│   │   └── stream
│   │
│   └── laptop
│       ├── queue
│       ├── pending
│       └── stream
│
└── admin
```

Очень важно понимать разделение:

```text
SQLite = постоянные данные

RAM/runtime = текущие соединения и выполняемые запросы
```

В SQLite храним:

```text
агентов
ключи
OAuth clients
permissions
access tokens
```

Не нужно хранить в SQLite:

```text
Go channels
активные HTTP streams
pending response channels
context.CancelFunc
```

Они существуют только в оперативной памяти.

---

# 6. Предлагаемая структура файлов

После переделки я рекомендую получить:

```text
cmd/gate/
├── main.go
├── server.go
│
├── models.go
├── store.go
├── secrets.go
├── runtime.go
├── auth.go
│
├── mcp.go
├── agent.go
├── oauth.go
│
├── admin.go
├── admin_templates.go
│
├── http_helpers.go
├── tool_policy.go
│
├── templates/
│   ├── tool-card.html
│   ├── admin-index.html
│   ├── admin-agent.html
│   └── admin-created.html
│
└── *_test.go
```

Не обязательно строго соблюдать названия, но разделение ответственности очень желательно.

---

# 7. Добавляем SQLite

## 7.1. Почему SQLite

Для нашей задачи PostgreSQL избыточен.

У нас:

```text
1 gate
небольшое количество агентов
небольшое количество записей
один Docker volume
```

SQLite подходит отлично.

Чтобы не тащить CGO и C-компилятор в Docker, удобно использовать:

```text
modernc.org/sqlite
```

Этот драйвер предоставляет `database/sql` SQLite driver без CGO.

---

# 8. Добавляем зависимость

В корне проекта:

```bash
go get modernc.org/sqlite
```

Затем:

```bash
go mod tidy
```

После этого должны измениться:

```text
go.mod
go.sum
```

Проверь:

```bash
git diff -- go.mod go.sum
```

Не редактируй `go.sum` руками.

---

# 9. Создаём models.go

Создай:

```text
cmd/gate/models.go
```

Пример модели:

```go
package main

import "time"

type Agent struct {
	ID        string
	Name      string
	Enabled   bool

	AgentTokenHash       string
	OAuthClientID        string
	OAuthClientSecretHash string

	AllowedTools string
	DeniedTools  string

	CreatedAt  time.Time
	UpdatedAt  time.Time
	LastSeenAt *time.Time
}

type AccessToken struct {
	TokenHash string
	AgentID   string
	ExpiresAt time.Time
	CreatedAt time.Time
}
```

Почему не хранить `AgentToken`?

Потому что в базе не должен лежать оригинальный:

```text
wc_agent_7h2HD...
```

Храним только:

```text
SHA-256(token)
```

---

# 10. Генерация секретов

Создай:

```text
cmd/gate/secrets.go
```

Пример:

```go
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

func generateSecret(prefix string) (string, error) {
	buf := make([]byte, 32)

	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random secret: %w", err)
	}

	value := base64.RawURLEncoding.EncodeToString(buf)

	return prefix + value, nil
}

func hashSecret(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
```

Теперь:

```go
generateSecret("wc_agent_")
```

может вернуть:

```text
wc_agent_GMF9gK......
```

Для OAuth:

```go
generateSecret("wc_oauth_")
```

Для MCP access token:

```go
generateSecret("wc_mcp_")
```

---

# 11. Никогда не логируй токены

Плохо:

```go
log.Printf("agent token=%s", token)
```

Плохо:

```go
log.Printf("authorization=%s", r.Header.Get("Authorization"))
```

Хорошо:

```go
log.Printf("agent authenticated id=%s", agent.ID)
```

Токен можно показать пользователю **один раз при создании/ротации**.

После этого в SQLite остаётся только hash.

---

# 12. Создаём store.go

Создай:

```text
cmd/gate/store.go
```

Начало:

```go
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type store struct {
	db *sql.DB
}
```

Создание:

```go
func newStore(path string) (*store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	s := &store{db: db}

	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}
```

---

# 13. Создаём таблицы

В `migrate()`:

```go
func (s *store) migrate(ctx context.Context) error {
	statements := []string{
		`
		CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,

			agent_token_hash TEXT NOT NULL UNIQUE,

			oauth_client_id TEXT NOT NULL UNIQUE,
			oauth_client_secret_hash TEXT NOT NULL,

			allowed_tools TEXT NOT NULL DEFAULT '',
			denied_tools TEXT NOT NULL DEFAULT '',

			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			last_seen_at DATETIME
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS access_tokens (
			token_hash TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL,

			FOREIGN KEY(agent_id) REFERENCES agents(id)
				ON DELETE CASCADE
		);
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_access_tokens_agent
		ON access_tokens(agent_id);
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_access_tokens_expires
		ON access_tokens(expires_at);
		`,
	}

	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return err
	}

	if _, err := s.db.ExecContext(ctx, `PRAGMA journal_mode = WAL`); err != nil {
		return err
	}

	if _, err := s.db.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		return err
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("database migration: %w", err)
		}
	}

	return nil
}
```

На первой версии этого достаточно.

Позже можно сделать нормальную систему миграций:

```text
schema_migrations
001_initial.sql
002_add_agent_metadata.sql
...
```

Но сейчас главное — получить работающий MVP.

---

# 14. CRUD агента

В `store.go` нужны функции:

```go
CreateAgent()
ListAgents()
GetAgent()
FindAgentByAgentTokenHash()
FindAgentByOAuthClientID()
FindAgentByAccessTokenHash()

SetAgentEnabled()
UpdateAgentTokenHash()
UpdateOAuthSecretHash()
UpdateToolPolicy()
DeleteAgent()

CreateAccessToken()
RevokeAccessTokens()
DeleteExpiredAccessTokens()

UpdateLastSeen()
```

Не пиши огромную функцию `doEverything()`.

---

# 15. Пример CreateAgent

Например:

```go
func (s *store) CreateAgent(ctx context.Context, agent Agent) error {
	_, err := s.db.ExecContext(
		ctx,
		`
		INSERT INTO agents (
			id,
			name,
			enabled,
			agent_token_hash,
			oauth_client_id,
			oauth_client_secret_hash,
			allowed_tools,
			denied_tools,
			created_at,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
		agent.ID,
		agent.Name,
		agent.Enabled,
		agent.AgentTokenHash,
		agent.OAuthClientID,
		agent.OAuthClientSecretHash,
		agent.AllowedTools,
		agent.DeniedTools,
		agent.CreatedAt,
		agent.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}

	return nil
}
```

---

# 16. Никогда не собирай SQL через строки пользователя

Нельзя:

```go
query := "SELECT * FROM agents WHERE id = '" + id + "'"
```

Нужно:

```go
row := s.db.QueryRowContext(
	ctx,
	`SELECT ... FROM agents WHERE id = ?`,
	id,
)
```

Это защищает от SQL injection.

---

# 17. Создаём runtime.go

Теперь самая важная часть.

Создай:

```text
cmd/gate/runtime.go
```

Новая структура:

```go
package main

import (
	"context"
	"sync"
	"time"

	"webcodex/internal/protocol"
)

type activeAgentStream struct {
	cancel context.CancelFunc
}

type agentRuntime struct {
	id string

	queue chan protocol.AgentRequest

	mu      sync.Mutex
	pending map[string]chan protocol.AgentResponse
	stream  *activeAgentStream

	lastSeen time.Time
}
```

Конструктор:

```go
func newAgentRuntime(id string) *agentRuntime {
	return &agentRuntime{
		id:      id,
		queue:   make(chan protocol.AgentRequest, 128),
		pending: make(map[string]chan protocol.AgentResponse),
	}
}
```

---

# 18. Server теперь хранит map runtime'ов

Текущий `server` содержит single-agent поля.

Переделываем концептуально в:

```go
type server struct {
	publicURL string
	timeout   time.Duration
	toolCards bool

	store *store

	runtimeMu sync.RWMutex
	runtimes  map[string]*agentRuntime

	adminUser     string
	adminPassword string
	adminCSRF     string

	oauthMu    sync.Mutex
	oauthCodes map[string]oauthCode
}
```

Удаляем из `server`:

```go
publicToken string
agentToken string

queue chan protocol.AgentRequest
pending map[string]chan protocol.AgentResponse
stream *activeAgentStream
```

---

# 19. Новый newServer()

Концепция:

```go
func newServer() (*server, error) {
	dbPath := env("WEBCODEX_DB_PATH", "/data/webcodex.db")

	db, err := newStore(dbPath)
	if err != nil {
		return nil, err
	}

	csrf, err := generateSecret("csrf_")
	if err != nil {
		db.db.Close()
		return nil, err
	}

	srv := &server{
		publicURL: strings.TrimRight(
			env("WEBCODEX_PUBLIC_URL", ""),
			"/",
		),

		timeout: durationEnv(
			"WEBCODEX_CALL_TIMEOUT",
			2*time.Minute,
		),

		toolCards: boolEnv(
			"WEBCODEX_TOOL_CARDS",
			false,
		),

		store: db,

		runtimes: make(map[string]*agentRuntime),

		adminUser: env(
			"WEBCODEX_ADMIN_USER",
			"admin",
		),

		adminPassword: env(
			"WEBCODEX_ADMIN_PASSWORD",
			"",
		),

		adminCSRF: csrf,

		oauthCodes: make(map[string]oauthCode),
	}

	if srv.adminPassword == "" {
		return nil, errors.New(
			"WEBCODEX_ADMIN_PASSWORD is required",
		)
	}

	if srv.publicURL == "" {
		srv.publicURL =
			"http://" +
				env(
					"WEBCODEX_ADDR",
					"127.0.0.1:8080",
				)
	}

	return srv, nil
}
```

Теперь при первом запуске БД может быть полностью пустой.

Это нормально.

Первого агента создаём через `/admin`.

---

# 20. Runtime создаём лениво

Не обязательно создавать runtime для каждой записи SQLite при запуске.

Можно:

```go
func (s *server) runtimeFor(agentID string) *agentRuntime {
	s.runtimeMu.RLock()
	rt := s.runtimes[agentID]
	s.runtimeMu.RUnlock()

	if rt != nil {
		return rt
	}

	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()

	if rt = s.runtimes[agentID]; rt != nil {
		return rt
	}

	rt = newAgentRuntime(agentID)
	s.runtimes[agentID] = rt

	return rt
}
```

Обрати внимание на повторную проверку после получения `Lock()`.

Без неё две goroutine теоретически могут одновременно создать два runtime для одного агента.

---

# 21. Переносим single-agent методы в agentRuntime

Текущие:

```go
activateAgentStream()
deactivateAgentStream()
callAgent()
enqueue()
forget()
```

должны работать не с:

```go
*server
```

а с:

```go
*agentRuntime
```

Например:

```go
func (rt *agentRuntime) activateAgentStream(
	parent context.Context,
) (
	context.Context,
	*activeAgentStream,
	bool,
) {
	streamCtx, cancel := context.WithCancel(parent)

	stream := &activeAgentStream{
		cancel: cancel,
	}

	rt.mu.Lock()

	previous := rt.stream
	rt.stream = stream
	rt.lastSeen = time.Now()

	rt.mu.Unlock()

	if previous != nil {
		previous.cancel()
	}

	return streamCtx, stream, previous != nil
}
```

Это очень важное поведение.

Мы **сохраняем правило**:

```text
один agent_id → один активный stream
```

Но теперь:

```text
home → stream HOME

work → stream WORK

laptop → stream LAPTOP
```

Одновременное существование разных агентов разрешено.

Если второй процесс подключится с тем же `home` token:

```text
старый HOME stream
        ↓
cancel

новый HOME stream
        ↓
active
```

Это разумное поведение.

---

# 22. Добавляем online status

В runtime:

```go
func (rt *agentRuntime) isOnline() bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	return rt.stream != nil
}
```

И:

```go
func (rt *agentRuntime) touch() {
	rt.mu.Lock()
	rt.lastSeen = time.Now()
	rt.mu.Unlock()
}
```

Можно отдельно сделать:

```go
func (rt *agentRuntime) status() (
	online bool,
	lastSeen time.Time,
) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	return rt.stream != nil, rt.lastSeen
}
```

---

# 23. Bearer token теперь должен возвращать агента

Сейчас helper:

```go
bearerOK(r, token)
```

просто сравнивает один токен.

Нам этого недостаточно.

Создай в `auth.go`:

```go
func bearerToken(r *http.Request) string {
	value := strings.TrimSpace(
		r.Header.Get("Authorization"),
	)

	const prefix = "Bearer "

	if !strings.HasPrefix(value, prefix) {
		return ""
	}

	return strings.TrimSpace(
		strings.TrimPrefix(value, prefix),
	)
}
```

---

# 24. Авторизация Windows agent

Добавь:

```go
func (s *server) authenticateAgent(
	r *http.Request,
) (*Agent, *agentRuntime, error) {
	token := bearerToken(r)

	if token == "" {
		return nil, nil, errors.New("missing bearer token")
	}

	hash := hashSecret(token)

	agent, err :=
		s.store.FindAgentByAgentTokenHash(
			r.Context(),
			hash,
		)

	if err != nil {
		return nil, nil, errors.New("invalid agent token")
	}

	if !agent.Enabled {
		return nil, nil, errors.New("agent disabled")
	}

	rt := s.runtimeFor(agent.ID)

	return agent, rt, nil
}
```

Теперь bearer token автоматически определяет:

```text
wc_agent_ABC
      ↓
SHA-256
      ↓
SQLite
      ↓
agent_id = home
      ↓
runtimeFor("home")
```

Никакой `X-Agent-ID` нам не нужен.

---

# 25. Авторизация MCP

Нужно аналогично:

```go
func (s *server) authenticateMCP(
	r *http.Request,
) (*Agent, *agentRuntime, error)
```

Но здесь token ищем не в `agents`.

Ищем:

```text
access_tokens
```

Пример логики:

```text
Bearer wc_mcp_xxx
        ↓
hash
        ↓
access_tokens.token_hash
        ↓
agent_id
        ↓
agents
        ↓
enabled?
        ↓
runtime
```

SQL удобно сделать `JOIN`.

Например:

```sql
SELECT
    a.id,
    a.name,
    a.enabled,
    a.allowed_tools,
    a.denied_tools,
    t.expires_at
FROM access_tokens t
JOIN agents a ON a.id = t.agent_id
WHERE t.token_hash = ?
LIMIT 1
```

Затем проверяем:

```go
if time.Now().After(expiresAt) {
	return ..., errors.New("access token expired")
}
```

---

# 26. Переделываем handleAgentStream()

Сейчас endpoint использует один `s.agentToken`.

Было концептуально:

```go
if !bearerOK(r, s.agentToken) {
	...
}
```

Станет:

```go
agent, rt, err := s.authenticateAgent(r)

if err != nil {
	http.Error(
		w,
		"unauthorized",
		http.StatusUnauthorized,
	)
	return
}
```

Дальше:

```go
streamCtx, stream, replaced :=
	rt.activateAgentStream(r.Context())
```

Лог:

```go
log.Printf(
	"agent stream connected agent=%s replaced=%t",
	agent.ID,
	replaced,
)
```

И главное:

```go
case request := <-rt.queue:
```

а не:

```go
case request := <-s.queue:
```

---

# 27. Переделываем handleAgentResult()

Сейчас результат ищется в общем:

```go
s.pending
```

Нужно:

```go
agent, rt, err := s.authenticateAgent(r)
```

После декодирования:

```go
rt.mu.Lock()
resultCh := rt.pending[result.ID]
rt.mu.Unlock()
```

Это одновременно даёт важную изоляцию.

Допустим:

```text
HOME выполняет request ID A
WORK выполняет request ID B
```

Даже если WORK каким-то образом отправит:

```text
result.ID = A
```

мы будем искать:

```text
WORK.pending["A"]
```

а не глобальный:

```text
server.pending["A"]
```

Поэтому ответ одного агента не попадёт другому.

---

# 28. Переделываем callAgent()

Было:

```go
s.callAgent(r, body)
```

Сделай:

```go
rt.callAgent(
	r.Context(),
	body,
	s.timeout,
)
```

Например:

```go
func (rt *agentRuntime) callAgent(
	ctx context.Context,
	request json.RawMessage,
	timeout time.Duration,
) (
	protocol.AgentResponse,
	error,
) {
	id, err := randomID()
	if err != nil {
		return protocol.AgentResponse{},
			fmt.Errorf(
				"create request id: %w",
				err,
			)
	}

	resultCh :=
		make(chan protocol.AgentResponse, 1)

	rt.mu.Lock()
	rt.pending[id] = resultCh
	rt.mu.Unlock()

	defer rt.forget(id)

	err = rt.enqueue(
		ctx,
		protocol.AgentRequest{
			ID:      id,
			Request: request,
		},
	)

	if err != nil {
		return protocol.AgentResponse{}, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case result := <-resultCh:
		if result.Error != "" {
			return protocol.AgentResponse{},
				errors.New(result.Error)
		}

		return result, nil

	case <-timer.C:
		return protocol.AgentResponse{},
			errors.New("agent call timed out")

	case <-ctx.Done():
		return protocol.AgentResponse{},
			ctx.Err()
	}
}
```

---

# 29. Важная проблема текущего enqueue()

Текущий код возвращает сообщение:

```text
agent queue is full or no agent is connected
```

хотя фактически запись в channel сама по себе не проверяет наличие stream.

В новой версии я рекомендую явно проверять online.

Например:

```go
func (rt *agentRuntime) enqueue(
	ctx context.Context,
	request protocol.AgentRequest,
) error {
	rt.mu.Lock()
	online := rt.stream != nil
	rt.mu.Unlock()

	if !online {
		return errors.New("agent is offline")
	}

	select {
	case rt.queue <- request:
		return nil

	case <-ctx.Done():
		return errors.New("request cancelled")

	default:
		return errors.New("agent queue is full")
	}
}
```

Так ошибки будут намного понятнее.

---

# 30. Переделываем handleMCP()

Это центральный момент.

Сейчас:

```go
authorized := bearerOK(
	r,
	s.publicToken,
)
```

заменяем на получение конкретного агента.

Пример:

```go
agent, rt, authErr :=
	s.authenticateMCP(r)

authorized := authErr == nil
```

Для методов, требующих авторизации:

```go
if !authorized {
	s.writeMCPUnauthorized(w)
	http.Error(
		w,
		"unauthorized",
		http.StatusUnauthorized,
	)
	return
}
```

---

# 31. MCP должен отправляться только выбранному runtime

Было:

```go
s.enqueue(...)
```

Станет:

```go
rt.enqueue(...)
```

Было:

```go
resp, err := s.callAgent(r, body)
```

Станет:

```go
resp, err := rt.callAgent(
	r.Context(),
	body,
	s.timeout,
)
```

Логирование:

```go
log.Printf(
	"mcp request agent=%s method=%q id=%s",
	agent.ID,
	msg.Method,
	string(msg.ID),
)
```

---

# 32. Tool policy делаем per-agent

Сейчас policy хранится глобально в `server`, а `tools/call` вызывает:

```go
s.toolPolicy.allows(call.Name)
```

и `tools/list` фильтруется через server policy.

Нужно делать:

```go
policy := newToolPolicy(
	agent.AllowedTools,
	agent.DeniedTools,
)
```

Для `tools/call`:

```go
if !policy.allows(call.Name) {
	writeRPCError(
		w,
		msg.ID,
		-32602,
		"tool not allowed",
	)
	return
}
```

---

# 33. Переделай filterToolsList()

Сейчас:

```go
func (s *server) filterToolsList(...)
```

использует:

```go
s.toolPolicy
```

Нам удобнее:

```go
func filterToolsList(
	response json.RawMessage,
	policy toolPolicy,
	toolCards bool,
) (
	json.RawMessage,
	error,
)
```

И внутри:

```go
if !ok || policy.allows(name) {
	if toolCards {
		decorateToolDescriptor(
			toolObject,
			name,
		)
	}

	filtered =
		append(filtered, tool)
}
```

В `handleMCP`:

```go
response, err :=
	filterToolsList(
		resp.Response,
		policy,
		s.toolCards,
	)
```

---

# 34. OAuth — ключ к маршрутизации ChatGPT

Текущий upstream OAuth очень простой: один client ID/secret и один общий public Bearer token. Token endpoint возвращает `s.publicToken`.

Мы меняем модель на:

```text
OAuth client
      ↓
agent
      ↓
новый access token
      ↓
access_tokens
      ↓
agent_id
```

Например:

```text
client_id = wc_client_home
        ↓
agent home
        ↓
wc_mcp_xyz
```

---

# 35. agents table и OAuth

В каждой записи:

```text
id = home

oauth_client_id =
wc_client_home_...

oauth_client_secret_hash =
SHA256(wc_oauth_...)
```

`oauth_client_id` секретом не является.

Его можно хранить plaintext.

`oauth_client_secret` — секрет.

Храним hash.

---

# 36. Создаём временный OAuth authorization code

Нельзя продолжать использовать один фиксированный:

```text
code=webcodex
```

Лучше сделать временный random code.

Создай:

```go
type oauthCode struct {
	AgentID      string
	ClientID     string
	RedirectURI  string
	CodeChallenge string
	CodeChallengeMethod string
	ExpiresAt    time.Time
}
```

Server:

```go
oauthMu sync.Mutex

oauthCodes map[string]oauthCode
```

---

# 37. handleAuthorize()

Алгоритм:

```text
GET /oauth/authorize
        ↓
client_id
        ↓
найти enabled agent
        ↓
создать random authorization code
        ↓
сохранить его в oauthCodes
        ↓
redirect_uri?code=...
```

Пример:

```go
clientID :=
	r.URL.Query().Get("client_id")

agent, err :=
	s.store.FindAgentByOAuthClientID(
		r.Context(),
		clientID,
	)

if err != nil || !agent.Enabled {
	http.Error(
		w,
		"unknown client",
		http.StatusUnauthorized,
	)
	return
}
```

Генерируем:

```go
code, err :=
	generateSecret("wc_code_")
```

Запоминаем на 5 минут:

```go
entry := oauthCode{
	AgentID: agent.ID,
	ClientID: clientID,
	RedirectURI:
		r.URL.Query().Get("redirect_uri"),

	CodeChallenge:
		r.URL.Query().Get("code_challenge"),

	CodeChallengeMethod:
		r.URL.Query().Get(
			"code_challenge_method",
		),

	ExpiresAt:
		time.Now().Add(
			5 * time.Minute,
		),
}
```

---

# 38. Очень важно сохранить redirect_uri

Когда `/oauth/token` позже получит:

```text
code
redirect_uri
```

проверяй, что `redirect_uri` тот же, который был на authorize.

Иначе получится слишком слабый OAuth flow.

---

# 39. PKCE

Текущий OAuth metadata проекта сообщает поддержку:

```text
S256
plain
```

для `code_challenge_methods_supported`.

Поэтому если оставляем эту metadata, новая реализация должна действительно это проверять.

Для `S256`:

```go
func verifyPKCE(
	verifier string,
	challenge string,
	method string,
) bool {
	switch method {
	case "":
		return true

	case "plain":
		return verifier == challenge

	case "S256":
		sum :=
			sha256.Sum256(
				[]byte(verifier),
			)

		calculated :=
			base64.RawURLEncoding.
				EncodeToString(
					sum[:],
				)

		return calculated == challenge

	default:
		return false
	}
}
```

---

# 40. handleToken()

Алгоритм:

```text
POST /oauth/token
        ↓
получить client_id/client_secret
        ↓
найти agent по client_id
        ↓
проверить secret hash
        ↓
получить authorization code
        ↓
проверить:
    client ID
    expiry
    redirect_uri
    PKCE
        ↓
удалить code
        ↓
создать новый wc_mcp_ token
        ↓
записать HASH в access_tokens
        ↓
вернуть RAW token ChatGPT
```

---

# 41. Проверяем OAuth client secret

Например:

```go
secretHash :=
	hashSecret(clientSecret)

if secretHash !=
	agent.OAuthClientSecretHash {
	http.Error(
		w,
		"bad client secret",
		http.StatusUnauthorized,
	)
	return
}
```

Для ещё более аккуратной реализации можно использовать constant-time comparison.

---

# 42. Создаём MCP access token

```go
rawToken, err :=
	generateSecret("wc_mcp_")
```

Hash:

```go
tokenHash :=
	hashSecret(rawToken)
```

Expiration:

```go
expiresAt :=
	time.Now().Add(
		365 * 24 * time.Hour,
	)
```

В базу пишем:

```text
token_hash
agent_id
expires_at
created_at
```

А клиенту возвращаем:

```go
writeJSON(
	w,
	map[string]any{
		"access_token": rawToken,
		"token_type":   "Bearer",
		"expires_in":   31536000,
		"scope":        "mcp",
	},
)
```

Таким образом raw MCP token нигде постоянно не хранится.

---

# 43. После использования authorization code удаляй его

Нельзя использовать один authorization code дважды.

Когда `/oauth/token` успешно проверил code:

```go
s.oauthMu.Lock()

delete(
	s.oauthCodes,
	params["code"],
)

s.oauthMu.Unlock()
```

Лучше удалить его непосредственно при получении/валидации атомарно, чтобы два параллельных запроса не смогли обменять один code.

---

# 44. Добавляем админку

Админка будет серверным HTML.

Не React.

Не Vue.

Не Node.js.

Используем:

```text
html/template
Go handlers
обычные HTML forms
минимальный CSS
```

Это резко упрощает:

```text
сборку
Docker
deploy
security surface
обновления
```

---

# 45. Админские маршруты

В `routes()` добавь концептуально:

```go
mux.HandleFunc(
	"GET /admin",
	s.adminAuth(
		s.handleAdminIndex,
	),
)

mux.HandleFunc(
	"POST /admin/agents",
	s.adminAuth(
		s.handleAdminCreateAgent,
	),
)

mux.HandleFunc(
	"GET /admin/agents/{id}",
	s.adminAuth(
		s.handleAdminAgent,
	),
)

mux.HandleFunc(
	"POST /admin/agents/{id}/toggle",
	s.adminAuth(
		s.handleAdminToggleAgent,
	),
)

mux.HandleFunc(
	"POST /admin/agents/{id}/rotate-agent-token",
	s.adminAuth(
		s.handleAdminRotateAgentToken,
	),
)

mux.HandleFunc(
	"POST /admin/agents/{id}/rotate-oauth-secret",
	s.adminAuth(
		s.handleAdminRotateOAuthSecret,
	),
)

mux.HandleFunc(
	"POST /admin/agents/{id}/revoke-access",
	s.adminAuth(
		s.handleAdminRevokeAccess,
	),
)

mux.HandleFunc(
	"POST /admin/agents/{id}/policy",
	s.adminAuth(
		s.handleAdminPolicy,
	),
)
```

Go module проекта использует Go 1.23, поэтому можно использовать новые pattern-based маршруты стандартного `http.ServeMux`.

---

# 46. Для первой версии — HTTP Basic Auth

Чтобы не превращать задачу в разработку полноценной auth-системы, для `/admin` можно использовать HTTP Basic Auth.

`.env`:

```env
WEBCODEX_ADMIN_USER=admin
WEBCODEX_ADMIN_PASSWORD=ОЧЕНЬ_ДЛИННЫЙ_ПАРОЛЬ
```

Middleware:

```go
func (s *server) adminAuth(
	next http.HandlerFunc,
) http.HandlerFunc {
	return func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		user, pass, ok :=
			r.BasicAuth()

		if !ok ||
			user != s.adminUser ||
			pass != s.adminPassword {

			w.Header().Set(
				"WWW-Authenticate",
				`Basic realm="WebCodex Admin"`,
			)

			http.Error(
				w,
				"unauthorized",
				http.StatusUnauthorized,
			)

			return
		}

		next(w, r)
	}
}
```

Админка обязательно должна находиться только за HTTPS.

---

# 47. Добавляем CSRF защиту

Даже с Basic Auth POST-формы желательно защищать.

При запуске gate мы уже создали:

```go
adminCSRF
```

В каждую HTML form:

```html
<input
    type="hidden"
    name="csrf"
    value="{{.CSRF}}"
>
```

Перед обработкой POST:

```go
if r.FormValue("csrf") != s.adminCSRF {
	http.Error(
		w,
		"bad csrf token",
		http.StatusForbidden,
	)
	return
}
```

Лучше сравнивать constant-time, но для первой версии главное не пропустить сам механизм.

---

# 48. HTML templates встроить в бинарник

Создай:

```text
cmd/gate/admin_templates.go
```

Например:

```go
package main

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html
var templateFS embed.FS

func parseAdminTemplates() (
	*template.Template,
	error,
) {
	return template.ParseFS(
		templateFS,
		"templates/*.html",
	)
}
```

У тебя уже есть директория `cmd/gate/templates` в upstream, поэтому можно использовать её же.

---

# 49. Главная страница admin

Пример визуально:

```text
WebCodex Gate
────────────────────────────────────────

Agents

● Home PC
  ID: home
  Online
  Last seen: now
  [Open]

● Work PC
  ID: work
  Offline
  Last seen: 2h ago
  [Open]


Add agent
────────────────────────────────────────

ID:
[ home-laptop       ]

Name:
[ Home laptop       ]

[ Create agent ]
```

---

# 50. Create Agent

Пользователь вводит:

```text
ID = home
Name = Home PC
```

ID лучше ограничить:

```text
a-z
0-9
-
_
```

Например regex:

```go
^[a-z0-9][a-z0-9_-]{0,63}$
```

Не разрешай:

```text
пробелы
/
\
?
#
..
```

---

# 51. Генерация credentials при создании

Handler делает:

```text
agent token
OAuth client ID
OAuth secret
```

Пример:

```go
agentToken, err :=
	generateSecret("wc_agent_")

oauthClientID, err :=
	generateSecret("wc_client_")

oauthSecret, err :=
	generateSecret("wc_oauth_")
```

В SQLite:

```text
agentTokenHash
OAuthClientID plaintext
OAuthSecretHash
```

На HTML странице показываем RAW значения.

---

# 52. Секреты показываем только один раз

Страница после создания:

```text
Agent created successfully

Agent ID:
home

Windows agent token:
wc_agent_ABC...

OAuth Client ID:
wc_client_XYZ...

OAuth Client Secret:
wc_oauth_DEF...

SAVE THESE VALUES NOW.
```

Очень важно:

после обновления страницы сервер **не должен уметь снова показать старый secret**.

Потому что в базе его уже нет.

Если пользователь забыл secret:

```text
Rotate
```

---

# 53. Конфигурация Windows agent

Страница должна сразу показать готовый пример.

PowerShell:

```powershell
$env:WEBCODEX_GATE_URL="https://mcp.example.com"
$env:WEBCODEX_AGENT_TOKEN="wc_agent_..."
.\webcodex-agent.exe
```

Это совместимо с существующей моделью конфигурации WebCodex, где агент принимает `WEBCODEX_GATE_URL` и `WEBCODEX_AGENT_TOKEN`.

---

# 54. Страница ChatGPT configuration

Покажи:

```text
Server URL:
https://mcp.example.com/mcp

Authorization URL:
https://mcp.example.com/oauth/authorize

Token URL:
https://mcp.example.com/oauth/token

Client ID:
wc_client_...

Client Secret:
wc_oauth_...
```

Именно такие MCP/OAuth endpoints использует существующий WebCodex.

---

# 55. Rotate Agent Token

При нажатии:

```text
Rotate Agent Token
```

сервер:

```text
1. создаёт новый token
2. пишет новый HASH в SQLite
3. отключает текущий agent stream
4. показывает новый RAW token
```

Отключение stream важно.

Иначе компьютер со старым token уже прошёл authentication и продолжит сидеть в существующем HTTP connection.

Добавь:

```go
func (rt *agentRuntime) disconnect() {
	rt.mu.Lock()

	stream := rt.stream

	rt.mu.Unlock()

	if stream != nil {
		stream.cancel()
	}
}
```

---

# 56. Disable Agent

При:

```text
enabled = false
```

нужно:

```text
UPDATE agents
SET enabled = 0
```

и:

```go
rt.disconnect()
```

После этого:

```text
старый agent stream закрыт
новый stream не авторизуется
MCP access tokens не работают
OAuth client не работает
```

Это должна быть единая логика.

---

# 57. Rotate OAuth Secret

При ротации:

```text
1. создать новый OAuth secret
2. заменить hash
3. удалить все access_tokens этого агента
4. показать новый secret один раз
```

Почему удалить access tokens?

Потому что иначе смена OAuth secret не отзовёт уже выданные MCP credentials.

---

# 58. Revoke MCP Access

Отдельная кнопка:

```text
Revoke all MCP sessions
```

делает:

```sql
DELETE FROM access_tokens
WHERE agent_id = ?
```

После этого существующий ChatGPT connector должен снова пройти OAuth.

---

# 59. Tool permissions

Страница агента:

```text
Allowed tools:
exec_command,read_file,write_file

Denied tools:
apply_patch
```

Можно начать просто с двух `<textarea>`.

Например:

```html
<label>Allowed tools</label>
<textarea name="allowed_tools">
{{.Agent.AllowedTools}}
</textarea>

<label>Denied tools</label>
<textarea name="denied_tools">
{{.Agent.DeniedTools}}
</textarea>
```

Не нужно сразу делать красивый список всех MCP tools.

---

# 60. Online/offline

Для admin index:

```go
rt := s.runtimeFor(agent.ID)

online, lastSeen :=
	rt.status()
```

Но будь осторожен.

Вызов `runtimeFor()` для offline agent создаст пустой runtime.

Это не ошибка, но можно сделать отдельный:

```go
func (s *server) existingRuntime(
	id string,
) *agentRuntime
```

который runtime не создаёт.

---

# 61. Last seen лучше также сохранять в SQLite

Каждые 5 секунд писать heartbeat в SQLite не надо.

Это лишние записи.

Достаточно обновлять:

```text
при подключении
при отключении
при получении result
```

Например:

```go
go s.store.UpdateLastSeen(
	context.Background(),
	agent.ID,
	time.Now(),
)
```

Для первой версии можешь делать синхронно.

---

# 62. Health endpoint

Существующий:

```text
GET /healthz
```

возвращает `204`.

Оставь его.

Можно улучшить:

```go
mux.HandleFunc(
	"GET /healthz",
	func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		ctx, cancel :=
			context.WithTimeout(
				r.Context(),
				time.Second,
			)

		defer cancel()

		if err :=
			s.store.db.PingContext(ctx);
			err != nil {

			http.Error(
				w,
				"database unavailable",
				http.StatusServiceUnavailable,
			)

			return
		}

		w.WriteHeader(
			http.StatusNoContent,
		)
	},
)
```

Теперь Docker healthcheck проверяет не только HTTP server, но и SQLite.

---

# 63. После первого большого этапа — остановись

До админки сначала добейся:

```text
SQLite работает
multi-agent работает
два agent streams работают одновременно
MCP token A → agent A
MCP token B → agent B
```

Только потом UI.

---

# 64. Обязательные unit tests

Старый тест проверяет, что второй stream заменяет первый single-agent stream.

Теперь нужны новые тесты.

Минимум:

```text
TestSameAgentStreamReplacesPrevious

TestDifferentAgentsCanHaveStreams

TestAgentTokenResolvesCorrectAgent

TestInvalidAgentTokenRejected

TestDisabledAgentRejected

TestAccessTokenResolvesCorrectAgent

TestExpiredAccessTokenRejected

TestAgentAResultCannotCompleteAgentBRequest

TestToolPolicyPerAgent

TestOAuthClientResolvesAgent

TestOAuthBadSecretRejected

TestOAuthCodeSingleUse

TestOAuthCodeExpires

TestRotateAgentTokenDisconnectsStream
```

---

# 65. Самый важный multi-agent тест

Концептуально:

```go
func TestDifferentAgentsCanHaveStreams(
	t *testing.T,
) {
	home :=
		newAgentRuntime("home")

	work :=
		newAgentRuntime("work")

	homeCtx, homeStream, _ :=
		home.activateAgentStream(
			context.Background(),
		)

	defer home.deactivateAgentStream(
		homeStream,
	)

	workCtx, workStream, replaced :=
		work.activateAgentStream(
			context.Background(),
		)

	defer work.deactivateAgentStream(
		workStream,
	)

	if replaced {
		t.Fatal(
			"work stream replaced home stream",
		)
	}

	if homeCtx.Err() != nil {
		t.Fatal(
			"home stream was cancelled",
		)
	}

	if workCtx.Err() != nil {
		t.Fatal(
			"work stream was cancelled",
		)
	}
}
```

---

# 66. Проверяем race conditions

Обязательно:

```bash
go test -race ./cmd/gate
```

Особенно потому что у нас есть:

```text
goroutines
maps
channels
streams
pending requests
admin changes
```

Если `-race` показывает ошибку — не игнорировать.

---

# 67. Форматирование

После каждого изменения Go:

```bash
gofmt -w cmd/gate
```

Потом:

```bash
go test ./cmd/gate
```

Затем:

```bash
go vet ./cmd/gate
```

Затем:

```bash
go test -race ./cmd/gate
```

---

# 68. Полная локальная проверка

После реализации:

```bash
go test ./...
```

Затем:

```bash
go vet ./...
```

Затем:

```bash
go build -o bin/webcodex-gate ./cmd/gate
```

---

# 69. Локальный запуск без Docker

Создай временную директорию:

```bash
mkdir -p dev-data
```

Linux/macOS:

```bash
export WEBCODEX_ADDR=:8080
export WEBCODEX_PUBLIC_URL=http://localhost:8080
export WEBCODEX_DB_PATH=./dev-data/webcodex.db
export WEBCODEX_ADMIN_USER=admin
export WEBCODEX_ADMIN_PASSWORD=test-password-change-me

./bin/webcodex-gate
```

PowerShell:

```powershell
$env:WEBCODEX_ADDR=":8080"
$env:WEBCODEX_PUBLIC_URL="http://localhost:8080"
$env:WEBCODEX_DB_PATH="./dev-data/webcodex.db"
$env:WEBCODEX_ADMIN_USER="admin"
$env:WEBCODEX_ADMIN_PASSWORD="test-password-change-me"

.\bin\webcodex-gate.exe
```

---

# 70. Проверка health

```bash
curl -i http://localhost:8080/healthz
```

Ожидаем примерно:

```text
HTTP/1.1 204 No Content
```

---

# 71. Проверка admin

Открыть:

```text
http://localhost:8080/admin
```

Browser запросит:

```text
username
password
```

Вводим:

```text
admin
test-password-change-me
```

---

# 72. Создай двух тестовых агентов

Например:

```text
home
work
```

Сохрани credentials обоих.

---

# 73. Проверяем независимость agent stream

Можно запустить два Windows agent процесса с разными tokens.

Окно 1:

```powershell
$env:WEBCODEX_GATE_URL="http://SERVER"
$env:WEBCODEX_AGENT_TOKEN="TOKEN_HOME"

.\webcodex-agent.exe
```

Окно 2:

```powershell
$env:WEBCODEX_GATE_URL="http://SERVER"
$env:WEBCODEX_AGENT_TOKEN="TOKEN_WORK"

.\webcodex-agent.exe
```

В admin одновременно должны быть:

```text
HOME   Online
WORK   Online
```

Если подключение WORK делает HOME Offline — значит где-то остался глобальный `s.stream`.

---

# 74. Ищи оставшийся single-agent state

После переделки выполни:

```bash
grep -R "s\.queue" cmd/gate
```

```bash
grep -R "s\.pending" cmd/gate
```

```bash
grep -R "s\.stream" cmd/gate
```

```bash
grep -R "s\.agentToken" cmd/gate
```

```bash
grep -R "s\.publicToken" cmd/gate
```

В новой архитектуре таких обращений быть уже не должно.

---

# 75. Коммит multi-agent backend

Когда backend заработал:

```bash
git add .
```

```bash
git commit -m "feat(gate): add multi-agent routing"
```

Следующий commit:

```bash
git commit -m "feat(gate): add sqlite persistence"
```

Следующий:

```bash
git commit -m "feat(gate): add admin interface"
```

Лучше несколько понятных commits, чем один:

```text
update stuff
```

---

# 76. Теперь Docker

В upstream `deploy/` сейчас находятся systemd unit-файлы; готового Docker deployment там нет.

Мы добавим свой.

В корне репозитория:

```text
Dockerfile.gate
docker-compose.yml
Caddyfile
.dockerignore
.env.example
```

---

# 77. .dockerignore

Это особенно важно, потому что WebCodex содержит большой `third_party/codex`, который gate для сборки не нужен.

Создай:

```text
.dockerignore
```

Содержимое:

```dockerignore
.git
.gitignore

bin
dev-data
data

third_party

*.db
*.sqlite
*.sqlite3

.env

README*
```

Можно также исключить:

```text
cmd/agent
```

если он точно не нужен build dependency gate.

Но сначала убедись:

```bash
go list -deps ./cmd/gate
```

---

# 78. Dockerfile.gate

Создай:

```dockerfile
FROM golang:1.23-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./

RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 \
    GOOS=linux \
    go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/webcodex-gate \
    ./cmd/gate


FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && addgroup -S webcodex \
    && adduser -S -G webcodex webcodex \
    && mkdir -p /data \
    && chown -R webcodex:webcodex /data

WORKDIR /app

COPY --from=builder \
    /out/webcodex-gate \
    /app/webcodex-gate

USER webcodex

EXPOSE 8080

VOLUME ["/data"]

ENTRYPOINT ["/app/webcodex-gate"]
```

---

# 79. Почему multi-stage

Первый image:

```text
golang
```

нужен только для компиляции.

Финальный image содержит:

```text
Alpine
CA certificates
готовый бинарник
```

Не содержит:

```text
Go compiler
исходники
.git
Rust
Codex source
```

Это уменьшает image и attack surface.

---

# 80. Почему CGO_ENABLED=0

Мы используем CGo-free SQLite driver, поэтому можем собирать standalone Go binary без зависимости от системного `libsqlite3`. Сам `modernc.org/sqlite` описывается как CGo-free SQLite driver.

---

# 81. Собираем Docker image

В корне проекта:

```bash
docker build \
  -f Dockerfile.gate \
  -t webcodex-gate:local \
  .
```

Проверяем:

```bash
docker images | grep webcodex
```

---

# 82. Проверяем image вручную

Создаём volume:

```bash
docker volume create webcodex-test-data
```

Запускаем:

```bash
docker run --rm \
  -p 8080:8080 \
  -v webcodex-test-data:/data \
  -e WEBCODEX_ADDR=:8080 \
  -e WEBCODEX_PUBLIC_URL=http://localhost:8080 \
  -e WEBCODEX_DB_PATH=/data/webcodex.db \
  -e WEBCODEX_ADMIN_USER=admin \
  -e WEBCODEX_ADMIN_PASSWORD=test-password \
  webcodex-gate:local
```

В другом терминале:

```bash
curl -i http://localhost:8080/healthz
```

---

# 83. Проверяем SQLite persistence

Создай агента через admin.

Затем останови container.

Снова запусти тот же `docker run` с:

```text
-v webcodex-test-data:/data
```

Агент должен остаться.

Если исчез — значит SQLite записывался не в `/data`.

---

# 84. docker-compose.yml

Теперь создаём:

```yaml
services:
  gate:
    build:
      context: .
      dockerfile: Dockerfile.gate

    restart: unless-stopped

    environment:
      WEBCODEX_ADDR: ":8080"
      WEBCODEX_PUBLIC_URL: "https://${MCP_DOMAIN}"
      WEBCODEX_DB_PATH: "/data/webcodex.db"

      WEBCODEX_ADMIN_USER: "${WEBCODEX_ADMIN_USER}"
      WEBCODEX_ADMIN_PASSWORD: "${WEBCODEX_ADMIN_PASSWORD}"

      WEBCODEX_CALL_TIMEOUT: "2m"
      WEBCODEX_TOOL_CARDS: "false"

    volumes:
      - webcodex_data:/data

    expose:
      - "8080"

    healthcheck:
      test:
        [
          "CMD",
          "wget",
          "-q",
          "--spider",
          "http://127.0.0.1:8080/healthz"
        ]

      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s


  caddy:
    image: caddy:2-alpine

    restart: unless-stopped

    depends_on:
      gate:
        condition: service_healthy

    environment:
      MCP_DOMAIN: "${MCP_DOMAIN}"

    ports:
      - "80:80"
      - "443:443"

    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config


volumes:
  webcodex_data:
  caddy_data:
  caddy_config:
```

---

# 85. Почему gate не публикуем наружу

Обрати внимание:

```yaml
gate:
  expose:
    - "8080"
```

а не:

```yaml
ports:
  - "8080:8080"
```

То есть с интернета непосредственно:

```text
SERVER_IP:8080
```

недоступен.

Caddy ходит к нему внутри Docker network:

```text
gate:8080
```

---

# 86. Caddyfile

Создай:

```text
Caddyfile
```

Содержимое:

```caddy
{$MCP_DOMAIN} {
    encode zstd gzip

    reverse_proxy gate:8080
}
```

Caddy будет TLS reverse proxy.

Публичная схема:

```text
Internet
   │
   │ 443 HTTPS
   ▼
Caddy
   │
   │ HTTP Docker network
   ▼
gate:8080
```

---

# 87. .env.example

Создай:

```dotenv
MCP_DOMAIN=mcp.example.com

WEBCODEX_ADMIN_USER=admin
WEBCODEX_ADMIN_PASSWORD=CHANGE_ME_TO_A_LONG_RANDOM_PASSWORD
```

---

# 88. Настоящий .env

Создай:

```bash
cp .env.example .env
```

Впиши настоящий домен:

```env
MCP_DOMAIN=mcp.example.com
```

И длинный пароль.

Например пароль лучше сгенерировать:

```bash
openssl rand -base64 32
```

---

# 89. Никогда не коммить .env

Проверь `.gitignore`.

Там обязательно должно быть:

```gitignore
.env
data/
dev-data/
*.db
*.sqlite
*.sqlite3
```

Проверка:

```bash
git status
```

`.env` не должен появляться в staged/untracked files.

---

# 90. Проверка docker compose

Сначала:

```bash
docker compose config
```

Эта команда очень полезна.

Она покажет итоговую compose-конфигурацию после подстановки `.env`.

Если YAML сломан — узнаешь до запуска.

---

# 91. Сборка Compose

```bash
docker compose build
```

После успешной сборки:

```bash
docker compose up -d
```

---

# 92. Проверяем контейнеры

```bash
docker compose ps
```

Ожидаем:

```text
gate     Up / healthy
caddy    Up
```

---

# 93. Логи gate

```bash
docker compose logs gate
```

Live:

```bash
docker compose logs -f gate
```

---

# 94. Логи Caddy

```bash
docker compose logs caddy
```

Live:

```bash
docker compose logs -f caddy
```

---

# 95. DNS перед Caddy

До production запуска DNS запись:

```text
mcp.example.com
```

должна указывать:

```text
A → PUBLIC_IP_SERVER
```

Если используется IPv6:

```text
AAAA → IPv6
```

Иначе автоматический HTTPS нормально не поднимется.

---

# 96. Firewall

На сервере наружу нужны:

```text
22/tcp   SSH
80/tcp   HTTP
443/tcp  HTTPS
```

Порт:

```text
8080
```

наружу открывать не нужно.

---

# 97. Проверка HTTPS

После запуска:

```bash
curl -i https://mcp.example.com/healthz
```

Ожидаем:

```text
HTTP/2 204
```

или:

```text
HTTP/3 204
```

в зависимости от клиента.

---

# 98. Проверка admin production

Открыть:

```text
https://mcp.example.com/admin
```

Ни в коем случае не использовать production admin через:

```text
http://
```

---

# 99. Создаём первого production agent

В admin:

```text
ID:
home

Name:
Home PC
```

Нажимаем:

```text
Create
```

Сохраняем:

```text
Agent Token
OAuth Client ID
OAuth Client Secret
```

---

# 100. Windows configuration

На Windows:

```powershell
$env:WEBCODEX_GATE_URL="https://mcp.example.com"
$env:WEBCODEX_AGENT_TOKEN="wc_agent_..."

.\webcodex-agent.exe
```

После запуска admin должен показать:

```text
Home PC
Online
```

---

# 101. Создаём второго агента

Admin:

```text
ID:
work

Name:
Work PC
```

Получаем другой:

```text
wc_agent_...
wc_client_...
wc_oauth_...
```

Запускаем второй Windows-agent.

Теперь одновременно:

```text
HOME    Online
WORK    Online
```

---

# 102. Главная acceptance test

Это главный тест всей разработки.

Должно быть:

```text
ChatGPT connection HOME
        ↓
OAuth client HOME
        ↓
MCP token HOME
        ↓
agent HOME
```

И одновременно:

```text
ChatGPT connection WORK
        ↓
OAuth client WORK
        ↓
MCP token WORK
        ↓
agent WORK
```

Запрос HOME ни при каких обстоятельствах не должен появляться у WORK.

---

# 103. Добавь agent ID во все важные логи

Было:

```text
mcp request method=tools/call
```

Сделай:

```text
mcp request agent=home method=tools/call
```

Было:

```text
agent stream connected
```

Сделай:

```text
agent stream connected agent=home
```

Было:

```text
agent stream dispatch id=...
```

Сделай:

```text
agent stream dispatch agent=home id=...
```

При debugging это сэкономит огромное количество времени.

---

# 104. Но секреты в логи не добавлять

Лог:

```text
agent authenticated agent=home
```

правильно.

Лог:

```text
agent token=wc_agent_...
```

неправильно.

Лог:

```text
OAuth client=wc_client_...
```

допустим, потому что client ID не является secret.

OAuth Client Secret логировать нельзя.

MCP Bearer token логировать нельзя.

---

# 105. Graceful shutdown — рекомендованное улучшение

Текущий `main.go` использует простой `http.ListenAndServe`.

Для Docker лучше позже заменить на `http.Server` + shutdown.

Пример:

```go
httpServer := &http.Server{
	Addr:              addr,
	Handler:           srv.routes(),
	ReadHeaderTimeout: 10 * time.Second,
}
```

Signal:

```go
ctx, stop :=
	signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)

defer stop()
```

Server goroutine:

```go
go func() {
	if err :=
		httpServer.ListenAndServe();
		err != nil &&
		!errors.Is(
			err,
			http.ErrServerClosed,
		) {

		log.Fatalf(
			"listen: %v",
			err,
		)
	}
}()
```

Shutdown:

```go
<-ctx.Done()

shutdownCtx, cancel :=
	context.WithTimeout(
		context.Background(),
		10*time.Second,
	)

defer cancel()

_ = httpServer.Shutdown(
	shutdownCtx,
)
```

После этого:

```bash
docker compose stop
```

будет завершать gate аккуратно.

---

# 106. Не закрывай queue при shutdown

Очень распространённая junior-ошибка:

```go
close(rt.queue)
```

пока другие goroutine потенциально ещё могут писать туда.

Это может вызвать:

```text
panic: send on closed channel
```

Для нашей архитектуры достаточно отменять contexts/HTTP streams.

---

# 107. Очистка expired OAuth/MCP данных

Можно при старте:

```go
s.store.DeleteExpiredAccessTokens(
	context.Background(),
	time.Now(),
)
```

И затем периодически:

```text
каждый час
```

Но это не blocker для MVP.

---

# 108. Backup SQLite

Постоянные данные находятся в Docker volume:

```text
webcodex_data
```

Перед серьёзным обновлением делай backup.

Простой вариант:

```bash
docker compose stop gate
```

Затем копируй volume/database.

После backup:

```bash
docker compose start gate
```

Ещё лучше позже добавить SQLite-aware backup команду.

---

# 109. Никогда не удаляй volume без понимания

Опасная команда:

```bash
docker compose down -v
```

`-v` удалит volumes.

То есть может исчезнуть:

```text
webcodex.db
agents
credentials hashes
settings
```

Обычное:

```bash
docker compose down
```

volume сохраняет.

---

# 110. Обновление production

Порядок:

```bash
git pull
```

или checkout нужного commit.

Потом:

```bash
docker compose build gate
```

Проверяем:

```bash
docker compose up -d
```

Затем:

```bash
docker compose ps
```

И:

```bash
docker compose logs --tail=100 gate
```

---

# 111. Перед production обязательно

Выполни:

```bash
gofmt -w cmd/gate
```

```bash
go test ./...
```

```bash
go test -race ./cmd/gate
```

```bash
go vet ./...
```

```bash
docker compose build
```

```bash
docker compose config
```

---

# 112. Definition of Done

Работа считается завершённой только если выполнены все условия:

```text
[ ] go test ./... проходит

[ ] go vet ./... проходит

[ ] go test -race ./cmd/gate проходит

[ ] gate собирается локально

[ ] Docker image собирается

[ ] SQLite переживает restart container

[ ] /healthz работает

[ ] /admin защищён паролем

[ ] можно создать агента

[ ] Agent Token показывается один раз

[ ] OAuth secret показывается один раз

[ ] можно подключить HOME agent

[ ] можно подключить WORK agent одновременно

[ ] WORK не отключает HOME

[ ] MCP HOME отправляет команды только HOME

[ ] MCP WORK отправляет команды только WORK

[ ] disabled agent больше не подключается

[ ] rotate agent token отключает старый stream

[ ] старый agent token после rotate не работает

[ ] revoke access делает старый MCP token невалидным

[ ] tool policy HOME не влияет на WORK

[ ] токены не появляются в logs

[ ] порт 8080 не опубликован в интернет

[ ] production работает через HTTPS
```

---

# 113. Рекомендуемый порядок commits

Делай примерно так:

```text
1.
refactor(gate): extract agent runtime

2.
feat(gate): add sqlite store

3.
feat(gate): add multi-agent authentication

4.
feat(gate): route mcp requests per agent

5.
feat(gate): add multi-client oauth

6.
feat(gate): add admin UI

7.
test(gate): cover multi-agent isolation

8.
build(gate): add docker image

9.
deploy(gate): add caddy compose stack

10.
docs(gate): document multi-agent deployment
```

Так при проблеме можно понять, на каком этапе архитектура сломалась.

---

# 114. Самая важная мысль для разработчика

Не думай про систему как:

```text
один gate + много токенов
```

Думай:

```text
один gate
    │
    ├── logical agent HOME
    │      ├── auth
    │      ├── queue
    │      ├── pending
    │      ├── stream
    │      └── tool policy
    │
    ├── logical agent WORK
    │      ├── auth
    │      ├── queue
    │      ├── pending
    │      ├── stream
    │      └── tool policy
    │
    └── logical agent LAPTOP
           ├── auth
           ├── queue
           ├── pending
           ├── stream
           └── tool policy
```

**Главное правило архитектуры: всё, что относится к конкретному агенту, не должно оставаться глобальным полем `server`.**

Глобальными остаются только:

```text
HTTP server
SQLite
public URL
call timeout
admin authentication
runtime map
OAuth temporary-code registry
```

А конкретному agent принадлежат:

```text
Agent Token
OAuth Client
MCP access tokens
queue
pending
stream
permissions
enabled state
last seen
```

---

# 115. Что НЕ нужно менять в первой версии

Не трогай без необходимости:

```text
third_party/codex
Codex MCP implementation
Windows execution logic
protocol.AgentRequest
protocol.AgentResponse
tool cards
MCP JSON-RPC format
```

Наша задача — изменить **маршрутизацию gate**, а не переписать WebCodex целиком.

Upstream уже разделяет публичный gate и локальный agent; server лишь пересылает MCP-вызовы на машину с рабочим окружением.

---

# 116. Финальная архитектура

После выполнения инструкции должно получиться:

```text
                         INTERNET

                            │
                         HTTPS 443
                            │
                            ▼

                    ┌──────────────┐
                    │    Caddy     │
                    └──────┬───────┘
                           │
                         :8080
                           │
                           ▼

              ┌────────────────────────┐
              │    webcodex-gate       │
              │                        │
              │ /mcp                   │
              │ /oauth/*               │
              │ /agent/stream          │
              │ /agent/result          │
              │ /admin                 │
              │ /healthz               │
              │                        │
              │ SQLite                 │
              │ /data/webcodex.db      │
              │                        │
              │ Runtime Router         │
              └──┬─────────┬────────┬──┘
                 │         │        │
             HOME│     WORK│    VPS2│
                 │         │        │
                 ▼         ▼        ▼
             Agent #1   Agent #2  Agent #3
             Windows    Windows   Linux


ChatGPT HOME
     │
     └── OAuth HOME
              │
              └── MCP token
                       │
                       └── HOME runtime only


ChatGPT WORK
     │
     └── OAuth WORK
              │
              └── MCP token
                       │
                       └── WORK runtime only
```

Это и есть целевая версия WebCodex Gate, которую затем можно безопасно развивать дальше.