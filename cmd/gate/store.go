package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var (
	errAgentNotFound       = errors.New("agent not found")
	errAccessTokenNotFound = errors.New("access token not found")
)

type store struct {
	db *sql.DB
}

func newStore(path string) (*store, error) {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

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

func (s *store) Close() error {
	return s.db.Close()
}

func (s *store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = ON;`); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA journal_mode = WAL;`); err != nil {
		return fmt.Errorf("set wal mode: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA busy_timeout = 5000;`); err != nil {
		return fmt.Errorf("set busy timeout: %w", err)
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS agents (
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
		);`,
		`CREATE TABLE IF NOT EXISTS access_tokens (
			token_hash TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			FOREIGN KEY(agent_id) REFERENCES agents(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_access_tokens_agent ON access_tokens(agent_id);`,
		`CREATE INDEX IF NOT EXISTS idx_access_tokens_expires ON access_tokens(expires_at);`,
	}

	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("database migration failed: %w", err)
		}
	}
	return nil
}

func (s *store) CreateAgent(ctx context.Context, agent Agent) error {
	now := time.Now().UTC()
	if agent.CreatedAt.IsZero() {
		agent.CreatedAt = now
	}
	if agent.UpdatedAt.IsZero() {
		agent.UpdatedAt = now
	}

	enabledInt := 0
	if agent.Enabled {
		enabledInt = 1
	}

	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO agents (
			id, name, enabled, agent_token_hash,
			oauth_client_id, oauth_client_secret_hash,
			allowed_tools, denied_tools, created_at, updated_at, last_seen_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		agent.ID,
		agent.Name,
		enabledInt,
		agent.AgentTokenHash,
		agent.OAuthClientID,
		agent.OAuthClientSecretHash,
		agent.AllowedTools,
		agent.DeniedTools,
		agent.CreatedAt.UTC().Format(time.RFC3339Nano),
		agent.UpdatedAt.UTC().Format(time.RFC3339Nano),
		formatNullableTime(agent.LastSeenAt),
	)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	return nil
}

func (s *store) ListAgents(ctx context.Context) ([]Agent, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, name, enabled, agent_token_hash,
		        oauth_client_id, oauth_client_secret_hash,
		        allowed_tools, denied_tools, created_at, updated_at, last_seen_at
		 FROM agents ORDER BY name ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var a Agent
		var enabledInt int
		var createdAtStr, updatedAtStr string
		var lastSeenStr sql.NullString

		if err := rows.Scan(
			&a.ID,
			&a.Name,
			&enabledInt,
			&a.AgentTokenHash,
			&a.OAuthClientID,
			&a.OAuthClientSecretHash,
			&a.AllowedTools,
			&a.DeniedTools,
			&createdAtStr,
			&updatedAtStr,
			&lastSeenStr,
		); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		a.Enabled = enabledInt == 1
		a.CreatedAt = parseDBTime(createdAtStr)
		a.UpdatedAt = parseDBTime(updatedAtStr)
		if lastSeenStr.Valid && lastSeenStr.String != "" {
			t := parseDBTime(lastSeenStr.String)
			a.LastSeenAt = &t
		}
		agents = append(agents, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return agents, nil
}

func (s *store) GetAgent(ctx context.Context, id string) (*Agent, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, name, enabled, agent_token_hash,
		        oauth_client_id, oauth_client_secret_hash,
		        allowed_tools, denied_tools, created_at, updated_at, last_seen_at
		 FROM agents WHERE id = ? LIMIT 1`,
		id,
	)
	return scanAgent(row)
}

func (s *store) FindAgentByAgentTokenHash(ctx context.Context, hash string) (*Agent, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, name, enabled, agent_token_hash,
		        oauth_client_id, oauth_client_secret_hash,
		        allowed_tools, denied_tools, created_at, updated_at, last_seen_at
		 FROM agents WHERE agent_token_hash = ? LIMIT 1`,
		hash,
	)
	return scanAgent(row)
}

func (s *store) FindAgentByOAuthClientID(ctx context.Context, clientID string) (*Agent, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, name, enabled, agent_token_hash,
		        oauth_client_id, oauth_client_secret_hash,
		        allowed_tools, denied_tools, created_at, updated_at, last_seen_at
		 FROM agents WHERE oauth_client_id = ? LIMIT 1`,
		clientID,
	)
	return scanAgent(row)
}

func (s *store) FindAgentByAccessTokenHash(ctx context.Context, hash string) (*Agent, *AccessToken, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT a.id, a.name, a.enabled, a.agent_token_hash,
		        a.oauth_client_id, a.oauth_client_secret_hash,
		        a.allowed_tools, a.denied_tools, a.created_at, a.updated_at, a.last_seen_at,
		        t.token_hash, t.agent_id, t.expires_at, t.created_at
		 FROM access_tokens t
		 JOIN agents a ON a.id = t.agent_id
		 WHERE t.token_hash = ? LIMIT 1`,
		hash,
	)

	var a Agent
	var t AccessToken
	var enabledInt int
	var aCreatedAtStr, aUpdatedAtStr string
	var aLastSeenStr sql.NullString
	var tExpiresAtStr, tCreatedAtStr string

	if err := row.Scan(
		&a.ID,
		&a.Name,
		&enabledInt,
		&a.AgentTokenHash,
		&a.OAuthClientID,
		&a.OAuthClientSecretHash,
		&a.AllowedTools,
		&a.DeniedTools,
		&aCreatedAtStr,
		&aUpdatedAtStr,
		&aLastSeenStr,
		&t.TokenHash,
		&t.AgentID,
		&tExpiresAtStr,
		&tCreatedAtStr,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, errAccessTokenNotFound
		}
		return nil, nil, fmt.Errorf("find agent by access token hash: %w", err)
	}

	a.Enabled = enabledInt == 1
	a.CreatedAt = parseDBTime(aCreatedAtStr)
	a.UpdatedAt = parseDBTime(aUpdatedAtStr)
	if aLastSeenStr.Valid && aLastSeenStr.String != "" {
		lastSeen := parseDBTime(aLastSeenStr.String)
		a.LastSeenAt = &lastSeen
	}

	t.ExpiresAt = parseDBTime(tExpiresAtStr)
	t.CreatedAt = parseDBTime(tCreatedAtStr)

	return &a, &t, nil
}

func (s *store) SetAgentEnabled(ctx context.Context, id string, enabled bool) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(
		ctx,
		`UPDATE agents SET enabled = ?, updated_at = ? WHERE id = ?`,
		enabledInt,
		nowStr,
		id,
	)
	if err != nil {
		return fmt.Errorf("set agent enabled: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errAgentNotFound
	}
	return nil
}

func (s *store) UpdateAgentTokenHash(ctx context.Context, id string, tokenHash string) error {
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(
		ctx,
		`UPDATE agents SET agent_token_hash = ?, updated_at = ? WHERE id = ?`,
		tokenHash,
		nowStr,
		id,
	)
	if err != nil {
		return fmt.Errorf("update agent token hash: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errAgentNotFound
	}
	return nil
}

func (s *store) UpdateOAuthSecretHash(ctx context.Context, id string, secretHash string) error {
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(
		ctx,
		`UPDATE agents SET oauth_client_secret_hash = ?, updated_at = ? WHERE id = ?`,
		secretHash,
		nowStr,
		id,
	)
	if err != nil {
		return fmt.Errorf("update oauth secret hash: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errAgentNotFound
	}
	return nil
}

func (s *store) UpdateToolPolicy(ctx context.Context, id string, allowedTools, deniedTools string) error {
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(
		ctx,
		`UPDATE agents SET allowed_tools = ?, denied_tools = ?, updated_at = ? WHERE id = ?`,
		allowedTools,
		deniedTools,
		nowStr,
		id,
	)
	if err != nil {
		return fmt.Errorf("update tool policy: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errAgentNotFound
	}
	return nil
}

func (s *store) DeleteAgent(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM agents WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errAgentNotFound
	}
	return nil
}

func (s *store) CreateAccessToken(ctx context.Context, token AccessToken) error {
	now := time.Now().UTC()
	if token.CreatedAt.IsZero() {
		token.CreatedAt = now
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO access_tokens (token_hash, agent_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		token.TokenHash,
		token.AgentID,
		token.ExpiresAt.UTC().Format(time.RFC3339Nano),
		token.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("create access token: %w", err)
	}
	return nil
}

func (s *store) RevokeAccessTokens(ctx context.Context, agentID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM access_tokens WHERE agent_id = ?`, agentID)
	if err != nil {
		return fmt.Errorf("revoke access tokens: %w", err)
	}
	return nil
}

func (s *store) DeleteExpiredAccessTokens(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(
		ctx,
		`DELETE FROM access_tokens WHERE expires_at <= ?`,
		now.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("delete expired access tokens: %w", err)
	}
	return nil
}

func (s *store) UpdateLastSeen(ctx context.Context, agentID string, seenAt time.Time) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE agents SET last_seen_at = ? WHERE id = ?`,
		seenAt.UTC().Format(time.RFC3339Nano),
		agentID,
	)
	if err != nil {
		return fmt.Errorf("update last seen: %w", err)
	}
	return nil
}

func scanAgent(row *sql.Row) (*Agent, error) {
	var a Agent
	var enabledInt int
	var createdAtStr, updatedAtStr string
	var lastSeenStr sql.NullString

	if err := row.Scan(
		&a.ID,
		&a.Name,
		&enabledInt,
		&a.AgentTokenHash,
		&a.OAuthClientID,
		&a.OAuthClientSecretHash,
		&a.AllowedTools,
		&a.DeniedTools,
		&createdAtStr,
		&updatedAtStr,
		&lastSeenStr,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errAgentNotFound
		}
		return nil, fmt.Errorf("scan agent: %w", err)
	}

	a.Enabled = enabledInt == 1
	a.CreatedAt = parseDBTime(createdAtStr)
	a.UpdatedAt = parseDBTime(updatedAtStr)
	if lastSeenStr.Valid && lastSeenStr.String != "" {
		t := parseDBTime(lastSeenStr.String)
		a.LastSeenAt = &t
	}
	return &a, nil
}

func formatNullableTime(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseDBTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
