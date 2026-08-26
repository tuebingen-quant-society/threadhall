// Package auth owns human credentials, invite redemption, and browser sessions.
package auth

import "time"

const tokenBytes = 32

// User is the authenticated human identity exposed to other domains.
type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Admin     bool      `json:"admin"`
	CreatedAt time.Time `json:"created_at"`
}

// DirectoryUser is the deliberately narrow identity exposed for member discovery.
type DirectoryUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

// UserDirectory is a bounded alphabetical discovery result.
type UserDirectory struct {
	Users []DirectoryUser `json:"users"`
}

// FindUsers searches workspace members without exposing account metadata.
type FindUsers struct {
	RequesterID int64
	Query       string
	Limit       int
}

// Bootstrap is the one-time first-administrator command.
type Bootstrap struct {
	Username string
	Password string
}

// CreateUser redeems an invite for a member account.
type CreateUser struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	InviteToken string `json:"invite_token"`
}

// Login authenticates an existing username and password.
type Login struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Session contains the only copy of a newly issued raw browser token.
type Session struct {
	Token     string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	User      User      `json:"user"`
}

// Invite contains the only copy of a newly issued raw invite token.
type Invite struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}
