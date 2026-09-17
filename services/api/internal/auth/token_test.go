package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const testSecret = "01234567890123456789012345678901"

func TestIssueAndParse(t *testing.T) {
	manager, err := NewManager(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	token, err := manager.Issue("user-1", "tenant-a", RoleBuilder, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := manager.Parse(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "user-1" || claims.TenantID != "tenant-a" || claims.Role != RoleBuilder {
		t.Fatalf("unexpected claims: %#v", claims)
	}
}

func TestRejectsTamperedToken(t *testing.T) {
	manager, _ := NewManager(testSecret)
	token, _ := manager.Issue("user-1", "tenant-a", RoleAdmin, time.Hour)
	parts := strings.Split(token, ".")
	parts[0] += "x"
	if _, err := manager.Parse(strings.Join(parts, ".")); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected invalid token, got %v", err)
	}
}

func TestRejectsExpiredToken(t *testing.T) {
	manager, _ := NewManager(testSecret)
	now := time.Unix(1_700_000_000, 0)
	manager.now = func() time.Time { return now }
	token, err := manager.Issue("user-1", "tenant-a", RoleAdmin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return now.Add(2 * time.Minute) }
	if _, err := manager.Parse(token); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expected expired token, got %v", err)
	}
}

func TestSecretLength(t *testing.T) {
	if _, err := NewManager("short"); err == nil {
		t.Fatal("expected short secret to be rejected")
	}
}
