package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Role string

const (
	RoleAdmin     Role = "admin"
	RoleBuilder   Role = "builder"
	RoleRequester Role = "requester"
	RoleApprover  Role = "approver"
	RoleAuditor   Role = "auditor"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("expired token")
)

type Claims struct {
	Subject  string `json:"sub"`
	TenantID string `json:"tenantId"`
	Role     Role   `json:"role"`
	Expires  int64  `json:"exp"`
}

type Manager struct {
	secret []byte
	now    func() time.Time
}

func NewManager(secret string) (*Manager, error) {
	if len(secret) < 32 {
		return nil, errors.New("token secret must be at least 32 characters")
	}
	return &Manager{secret: []byte(secret), now: time.Now}, nil
}

func (m *Manager) Issue(subject, tenantID string, role Role, ttl time.Duration) (string, error) {
	if subject == "" || tenantID == "" || !ValidRole(role) || ttl <= 0 {
		return "", ErrInvalidToken
	}
	claims := Claims{Subject: subject, TenantID: tenantID, Role: role, Expires: m.now().Add(ttl).Unix()}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + m.sign(encoded), nil
}

func (m *Manager) Parse(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(m.sign(parts[0])), []byte(parts[1])) {
		return Claims{}, ErrInvalidToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if claims.Subject == "" || claims.TenantID == "" || !ValidRole(claims.Role) {
		return Claims{}, ErrInvalidToken
	}
	if m.now().Unix() >= claims.Expires {
		return Claims{}, ErrExpiredToken
	}
	return claims, nil
}

func (m *Manager) sign(payload string) string {
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func ValidRole(role Role) bool {
	switch role {
	case RoleAdmin, RoleBuilder, RoleRequester, RoleApprover, RoleAuditor:
		return true
	default:
		return false
	}
}

func Bearer(header string) (string, error) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", fmt.Errorf("%w: bearer token required", ErrInvalidToken)
	}
	return parts[1], nil
}
