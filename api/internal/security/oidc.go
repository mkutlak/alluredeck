package security

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"
	"golang.org/x/oauth2"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// OIDCUserInfo holds claims extracted from an OIDC ID token.
type OIDCUserInfo struct {
	Subject string
	Email   string
	Name    string
	Groups  []string
}

// OIDCExchanger is the interface for OIDC authorization code exchange operations.
// Implemented by OIDCProvider and test mocks.
type OIDCExchanger interface {
	AuthCodeURL(state, nonce, codeChallenge string) string
	// Exchange exchanges the authorization code, validates the PKCE verifier and nonce,
	// and returns the user info extracted from the ID token claims.
	Exchange(ctx context.Context, code, codeVerifier, nonce string) (*OIDCUserInfo, error)
}

// OIDCProvider wraps the go-oidc provider, verifier, and oauth2 config.
type OIDCProvider struct {
	provider   *gooidc.Provider
	verifier   *gooidc.IDTokenVerifier
	oauth2Cfg  oauth2.Config
	httpClient *http.Client // used for OIDC discovery + token exchange; wraps otelhttp transport
	logger     *zap.Logger
}

// NewOIDCProvider discovers the OIDC provider from IssuerURL and returns an OIDCProvider.
// Blocks until OIDC discovery completes. Returns an error if discovery fails.
// Outbound OIDC/OAuth2 HTTP requests are instrumented with OpenTelemetry via
// otelhttp.NewTransport so they appear as child spans of the originating request.
func NewOIDCProvider(ctx context.Context, cfg *config.OIDCConfig, logger *zap.Logger) (*OIDCProvider, error) {
	// Build an OTel-instrumented HTTP client for OIDC discovery and token exchange.
	otelClient := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	// Inject the client so go-oidc uses it for discovery.
	discoverCtx := context.WithValue(ctx, oauth2.HTTPClient, otelClient)

	provider, err := gooidc.NewProvider(discoverCtx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery failed for issuer %q: %w", cfg.IssuerURL, err)
	}

	verifierCfg := &gooidc.Config{ClientID: cfg.ClientID}
	verifier := provider.Verifier(verifierCfg)

	oauth2Cfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       cfg.Scopes,
	}

	return &OIDCProvider{
		provider:   provider,
		verifier:   verifier,
		oauth2Cfg:  oauth2Cfg,
		httpClient: otelClient,
		logger:     logger,
	}, nil
}

// AuthCodeURL builds the IdP authorization URL with PKCE S256 challenge.
func (p *OIDCProvider) AuthCodeURL(state, nonce, codeChallenge string) string {
	return p.oauth2Cfg.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
}

// Exchange exchanges an authorization code for an ID token using PKCE,
// validates the nonce to prevent replay attacks, and returns the extracted user info.
// The OTel-instrumented http.Client is injected via context so the token exchange
// request appears as a child span.
func (p *OIDCProvider) Exchange(ctx context.Context, code, codeVerifier, nonce string) (*OIDCUserInfo, error) {
	if p.httpClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, p.httpClient)
	}
	token, err := p.oauth2Cfg.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", codeVerifier))
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in token response")
	}

	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("ID token verification failed: %w", err)
	}

	// Validate nonce to prevent replay attacks.
	if idToken.Nonce != nonce {
		return nil, fmt.Errorf("nonce mismatch: expected %q", nonce)
	}

	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to extract ID token claims: %w", err)
	}
	return extractClaimsFromMap(claims, idToken.Subject, p.logger), nil
}

// ExtractClaims parses the ID token claims and returns an OIDCUserInfo.
// If Azure AD group overage is detected (_claim_names contains "groups"),
// it logs a warning and returns empty groups.
func (p *OIDCProvider) ExtractClaims(idToken *gooidc.IDToken) (*OIDCUserInfo, error) {
	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to extract ID token claims: %w", err)
	}
	return extractClaimsFromMap(claims, idToken.Subject, p.logger), nil
}

// extractClaimsFromMap extracts OIDCUserInfo from a raw claims map.
// Separated from ExtractClaims to allow unit testing without a real IDToken.
func extractClaimsFromMap(claims map[string]any, subject string, logger *zap.Logger) *OIDCUserInfo {
	info := &OIDCUserInfo{Subject: subject}

	if email, ok := claims["email"].(string); ok {
		info.Email = email
	}
	if name, ok := claims["name"].(string); ok {
		info.Name = name
	}

	// Detect Azure AD group overage: _claim_names contains "groups"
	if claimNames, ok := claims["_claim_names"].(map[string]any); ok {
		if _, hasGroups := claimNames["groups"]; hasGroups {
			if logger != nil {
				logger.Warn("OIDC: Azure AD group overage detected; groups claim omitted — configure app roles or use Graph API",
					zap.String("subject", subject))
			}
			return info
		}
	}

	// Extract groups from claims
	if rawGroups, ok := claims["groups"]; ok {
		switch v := rawGroups.(type) {
		case []any:
			for _, g := range v {
				if s, ok := g.(string); ok {
					info.Groups = append(info.Groups, s)
				}
			}
		case []string:
			info.Groups = v
		}
	}

	return info
}

// PKCEChallenge computes the S256 PKCE code challenge from a code verifier.
// codeVerifier must be a high-entropy random string (RFC 7636 §4.1).
func PKCEChallenge(codeVerifier string) string {
	h := sha256.Sum256([]byte(codeVerifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
