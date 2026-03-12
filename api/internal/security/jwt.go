package security

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// Sentinel errors for JWT validation.
var (
	ErrUnexpectedSigningMethod = errors.New("unexpected signing method")
	ErrInvalidTokenClaims      = errors.New("invalid token claims")
	ErrInvalidTokenType        = errors.New("invalid token type")
	ErrTokenRevoked            = errors.New("token has been revoked")
)

// BlacklistStore is the interface for persistent JWT revocation storage.
// Implemented by *store.BlacklistStore; an in-memory stub can be used in tests.
type BlacklistStore interface {
	AddToBlacklist(ctx context.Context, jti string, expiresAt time.Time) error
	IsBlacklisted(ctx context.Context, jti string) (bool, error)
	PruneExpired(ctx context.Context) (int64, error)
}

// JWTManager handles JWT token generation, validation, and revocation.
type JWTManager struct {
	cfg       *config.Config
	blacklist BlacklistStore
}

// NewJWTManager creates a new JWTManager backed by the given config and persistent blacklist store.
func NewJWTManager(cfg *config.Config, blacklist BlacklistStore) *JWTManager {
	return &JWTManager{
		cfg:       cfg,
		blacklist: blacklist,
	}
}

// GenerateTokens creates access and refresh tokens with JTI claims for revocation support.
// The role is embedded in the access token claims for RBAC enforcement (REVIEW #9).
func (m *JWTManager) GenerateTokens(username, role string) (accessToken, refreshToken string, err error) {
	accessJTI, err := generateJTI()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate access token JTI: %w", err)
	}
	refreshJTI, err := generateJTI()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate refresh token JTI: %w", err)
	}

	now := time.Now()

	accessClaims := jwt.MapClaims{
		"sub":  username,
		"role": role,
		"type": "access",
		"jti":  accessJTI,
		"exp":  jwt.NewNumericDate(now.Add(m.cfg.AccessTokenExpiry.Duration())),
		"iat":  jwt.NewNumericDate(now),
	}

	accessJWT := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessToken, err = accessJWT.SignedString([]byte(m.cfg.JWTSecret))
	if err != nil {
		return "", "", fmt.Errorf("failed to sign access token: %w", err)
	}

	refreshClaims := jwt.MapClaims{
		"sub":  username,
		"type": "refresh",
		"jti":  refreshJTI,
		"exp":  jwt.NewNumericDate(now.Add(m.cfg.RefreshTokenExpiry.Duration())),
		"iat":  jwt.NewNumericDate(now),
	}

	refreshJWT := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshToken, err = refreshJWT.SignedString([]byte(m.cfg.JWTSecret))
	if err != nil {
		return "", "", fmt.Errorf("failed to sign refresh token: %w", err)
	}

	return accessToken, refreshToken, nil
}

// ValidateToken parses and validates a token, returning claims for downstream use
func (m *JWTManager) ValidateToken(tokenString, expectedType string) (*jwt.Token, jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v: %w", token.Header["alg"], ErrUnexpectedSigningMethod)
		}
		return []byte(m.cfg.JWTSecret), nil
	})

	if err != nil {
		return nil, nil, fmt.Errorf("parse JWT token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, nil, ErrInvalidTokenClaims
	}

	tokenType, ok := claims["type"].(string)
	if !ok || tokenType != expectedType {
		return nil, nil, fmt.Errorf("invalid token type, expected %s: %w", expectedType, ErrInvalidTokenType)
	}

	// Check JTI against persistent blacklist (revoked tokens survive restarts).
	if jti, ok := claims["jti"].(string); ok {
		if m.IsBlacklisted(jti) {
			return nil, nil, ErrTokenRevoked
		}
	}

	return token, claims, nil
}

// AddToBlacklist adds a token JTI with its expiry time to the persistent revoked list.
func (m *JWTManager) AddToBlacklist(jti string, expiry time.Time) {
	if err := m.blacklist.AddToBlacklist(context.Background(), jti, expiry); err != nil {
		// Log but don't fail — a missed blacklist entry is a security issue but not a crash condition.
		// The token will expire naturally.
		_ = err
	}
}

// IsBlacklisted checks if a token JTI is currently revoked.
func (m *JWTManager) IsBlacklisted(jti string) bool {
	blacklisted, err := m.blacklist.IsBlacklisted(context.Background(), jti)
	if err != nil {
		// On DB error, treat as not blacklisted to avoid locking out all users.
		return false
	}
	return blacklisted
}

// StartCleanup starts a background goroutine that periodically removes expired
// JTIs from the persistent blacklist to prevent unbounded growth.
func (m *JWTManager) StartCleanup(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := m.blacklist.PruneExpired(ctx); err != nil {
					_ = err // non-fatal
				}
			}
		}
	}()
}

func generateJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate JTI random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}
