package security

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

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
	logger    *zap.Logger
}

// NewJWTManager creates a new JWTManager backed by the given config and persistent blacklist store.
func NewJWTManager(cfg *config.Config, blacklist BlacklistStore, logger *zap.Logger) *JWTManager {
	return &JWTManager{
		cfg:       cfg,
		blacklist: blacklist,
		logger:    logger,
	}
}

// GenerateTokens creates access and refresh tokens with JTI claims for revocation support.
// The role is embedded in the access token claims for RBAC enforcement (REVIEW #9).
// The optional provider parameter (default "local") is embedded as the "provider" claim
// so the Session endpoint can report the authentication source.
//
// This is a backwards-compatible wrapper around GenerateTokensForFamily that does NOT
// participate in refresh-token-family rotation. Production callers (Login, OIDC, Refresh)
// should use GenerateTokensForFamily directly so the refresh token gets a `fam` claim.
func (m *JWTManager) GenerateTokens(username, role string, provider ...string) (accessToken, refreshToken string, err error) {
	prov := "local"
	if len(provider) > 0 && provider[0] != "" {
		prov = provider[0]
	}
	access, refresh, _, _, err := m.GenerateTokensForFamily(username, role, prov, "")
	return access, refresh, err
}

// GenerateTokensForFamily creates access and refresh tokens and returns the JTIs of both
// so callers can persist them in a refresh-token-family record for rotation tracking.
//
// If familyID is non-empty, it is added to the refresh token as the `fam` claim. If empty,
// the refresh token is minted without a `fam` claim (legacy behavior used by GenerateTokens).
//
// The provider parameter is embedded as the "provider" claim in the access token.
func (m *JWTManager) GenerateTokensForFamily(username, role, provider, familyID string) (accessToken, refreshToken, accessJTI, refreshJTI string, err error) {
	if provider == "" {
		provider = "local"
	}

	accessJTI, err = generateJTI()
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to generate access token JTI: %w", err)
	}
	refreshJTI, err = generateJTI()
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to generate refresh token JTI: %w", err)
	}

	now := time.Now()

	accessClaims := jwt.MapClaims{
		"sub":      username,
		"role":     role,
		"provider": provider,
		"type":     "access",
		"jti":      accessJTI,
		"exp":      jwt.NewNumericDate(now.Add(m.cfg.AccessTokenExpiry.Duration())),
		"iat":      jwt.NewNumericDate(now),
	}

	accessJWT := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessToken, err = accessJWT.SignedString([]byte(m.cfg.JWTSecret))
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to sign access token: %w", err)
	}

	refreshClaims := jwt.MapClaims{
		"sub":  username,
		"type": "refresh",
		"jti":  refreshJTI,
		"exp":  jwt.NewNumericDate(now.Add(m.cfg.RefreshTokenExpiry.Duration())),
		"iat":  jwt.NewNumericDate(now),
	}
	if familyID != "" {
		refreshClaims["fam"] = familyID
	}

	refreshJWT := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshToken, err = refreshJWT.SignedString([]byte(m.cfg.JWTSecret))
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to sign refresh token: %w", err)
	}

	return accessToken, refreshToken, accessJTI, refreshJTI, nil
}

// NewFamilyID generates a fresh refresh-token-family ID using the same RNG as JTIs.
// This is exported so callers (Login handlers) can mint a family ID before calling
// GenerateTokensForFamily and persisting the family in the store.
func NewFamilyID() (string, error) {
	return generateJTI()
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
		m.logger.Error("failed to blacklist token", zap.String("jti", jti), zap.Error(err))
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
					m.logger.Error("failed to prune expired tokens", zap.Error(err))
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
