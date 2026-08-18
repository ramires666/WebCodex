package main

import "time"

// Agent represents a registered execution agent (e.g. Windows PC, Linux VPS).
type Agent struct {
	ID                    string     `json:"id"`
	Name                  string     `json:"name"`
	Enabled               bool       `json:"enabled"`
	AgentTokenHash        string     `json:"-"`
	OAuthClientID         string     `json:"oauth_client_id"`
	OAuthClientSecretHash string     `json:"-"`
	AllowedTools          string     `json:"allowed_tools"`
	DeniedTools           string     `json:"denied_tools"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	LastSeenAt            *time.Time `json:"last_seen_at,omitempty"`
}

// AccessToken represents an active issued bearer token for MCP clients (ChatGPT).
type AccessToken struct {
	TokenHash string    `json:"token_hash"`
	AgentID   string    `json:"agent_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// oauthCode represents a temporary single-use authorization code during OAuth flow.
type oauthCode struct {
	AgentID             string
	ClientID            string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	ExpiresAt           time.Time
}
