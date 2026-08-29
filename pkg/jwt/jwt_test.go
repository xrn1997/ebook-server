package jwt

import (
	"ebook-server/config"
	"testing"
	"time"
)

func setup() {
	config.AppConfig = &config.Config{
		JWT: config.JWTConfig{
			Secret:     "test-secret-key",
			ExpireHour: 1,
		},
	}
}

func TestGenerateToken(t *testing.T) {
	setup()

	token, err := GenerateToken(1, "testuser")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	if token == "" {
		t.Error("Token should not be empty")
	}

	// Token should have 3 parts separated by dots
	parts := splitToken(token)
	if len(parts) != 3 {
		t.Errorf("Token should have 3 parts, got %d", len(parts))
	}
}

func TestParseToken(t *testing.T) {
	setup()

	// Generate a token
	token, err := GenerateToken(1, "testuser")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Parse the token
	claims, err := ParseToken(token)
	if err != nil {
		t.Fatalf("Failed to parse token: %v", err)
	}

	if claims.UserID != 1 {
		t.Errorf("Expected UserID 1, got %d", claims.UserID)
	}

	if claims.Username != "testuser" {
		t.Errorf("Expected Username 'testuser', got '%s'", claims.Username)
	}
}

func TestParseTokenInvalid(t *testing.T) {
	setup()

	_, err := ParseToken("invalid-token")
	if err == nil {
		t.Error("Expected error for invalid token")
	}
}

func TestParseTokenWrongSecret(t *testing.T) {
	setup()

	// Generate token with different secret
	otherConfig := &config.Config{
		JWT: config.JWTConfig{
			Secret:     "other-secret",
			ExpireHour: 1,
		},
	}
	originalConfig := config.AppConfig
	config.AppConfig = otherConfig
	token, _ := GenerateToken(1, "testuser")
	config.AppConfig = originalConfig

	// Try to parse with original secret
	_, err := ParseToken(token)
	if err == nil {
		t.Error("Expected error for token with wrong secret")
	}
}

func TestTokenExpiration(t *testing.T) {
	// Setup with very short expiration
	config.AppConfig = &config.Config{
		JWT: config.JWTConfig{
			Secret:     "test-secret",
			ExpireHour: 0, // 0 hours = expired immediately
		},
	}

	token, err := GenerateToken(1, "testuser")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Wait a bit to ensure token expires
	time.Sleep(10 * time.Millisecond)

	_, err = ParseToken(token)
	if err == nil {
		t.Error("Expected error for expired token")
	}
}

func TestTokenClaims(t *testing.T) {
	setup()

	token, err := GenerateToken(123, "admin")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	claims, err := ParseToken(token)
	if err != nil {
		t.Fatalf("Failed to parse token: %v", err)
	}

	if claims.UserID != 123 {
		t.Errorf("Expected UserID 123, got %d", claims.UserID)
	}

	if claims.Username != "admin" {
		t.Errorf("Expected Username 'admin', got '%s'", claims.Username)
	}

	if claims.Issuer != "ebook-server" {
		t.Errorf("Expected Issuer 'ebook-server', got '%s'", claims.Issuer)
	}

	if claims.ExpiresAt == nil {
		t.Error("ExpiresAt should not be nil")
	}

	if claims.IssuedAt == nil {
		t.Error("IssuedAt should not be nil")
	}
}

func splitToken(token string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			parts = append(parts, token[start:i])
			start = i + 1
		}
	}
	parts = append(parts, token[start:])
	return parts
}
