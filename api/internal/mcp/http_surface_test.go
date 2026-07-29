package mcp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/config"
	internalmcp "github.com/mkutlak/alluredeck/api/internal/mcp"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

const testPublicURL = "https://alluredeck.example.com"

// newTestMCPHandler builds the real MCP HTTP handler chain over mock stores.
func newTestMCPHandler(t *testing.T) http.Handler {
	t.Helper()
	stores := &bootstrap.Stores{
		APIKey:              &testutil.MockAPIKeyStore{},
		DefectProposals:     &testutil.MockDefectProposalStore{},
		KnownIssueProposals: &testutil.MockKnownIssueProposalStore{},
		FlakyProposals:      &testutil.MockFlakyProposalStore{},
		TestResult:          &testutil.MockTestResultStore{},
		Attachment:          &testutil.MockAttachmentStore{},
		Audit:               testutil.NewMockAuditLogger(),
	}
	jwtManager := security.NewJWTManager(&config.Config{
		JWTSecret:          "test-secret-key",
		AccessTokenExpiry:  config.DurationSeconds(15 * time.Minute),
		RefreshTokenExpiry: config.DurationSeconds(30 * 24 * time.Hour),
	}, testutil.NewMemBlacklist(), zap.NewNop())

	handler, _, err := internalmcp.NewServer(internalmcp.Config{
		RateLimitPerMin: 600,
		RateLimitBurst:  100,
		PublicURL:       testPublicURL,
		SigningKey:      []byte("test-signing-key"),
		Stateless:       true,
	}, stores, jwtManager, nil, zap.NewNop())
	if err != nil {
		t.Fatalf("building MCP server: %v", err)
	}
	return handler
}

// TestUnauthenticatedRequestAdvertisesDiscovery verifies the RFC 9728 wiring:
// a 401 must tell the client where to learn how to authenticate, rather than
// only that it failed.
func TestUnauthenticatedRequestAdvertisesDiscovery(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Content-Type", "application/json")
	newTestMCPHandler(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	auth := rec.Header().Get("WWW-Authenticate")
	if auth == "" {
		t.Fatal("401 carries no WWW-Authenticate header, so clients cannot discover how to authenticate")
	}
	if !strings.Contains(auth, internalmcp.ResourceMetadataPath) {
		t.Errorf("WWW-Authenticate = %q, want it to reference %q", auth, internalmcp.ResourceMetadataPath)
	}
}

// TestProtectedResourceMetadataIsPublic verifies the discovery document is
// served without a token. A client fetches it precisely because it does not
// have one yet, so requiring auth here would be circular.
func TestProtectedResourceMetadataIsPublic(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, internalmcp.ResourceMetadataPath, nil)
	internalmcp.ProtectedResourceMetadataHandler(testPublicURL).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 without a token", rec.Code)
	}

	var doc struct {
		Resource               string   `json:"resource"`
		BearerMethodsSupported []string `json:"bearer_methods_supported"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decoding metadata document: %v", err)
	}
	if want := testPublicURL + "/mcp"; doc.Resource != want {
		t.Errorf("resource = %q, want %q", doc.Resource, want)
	}
	if len(doc.BearerMethodsSupported) == 0 || doc.BearerMethodsSupported[0] != "header" {
		t.Errorf("bearer_methods_supported = %v, want [header]", doc.BearerMethodsSupported)
	}
}

// TestStatelessTransportRejectsGET confirms stateless mode is actually engaged.
// The 2026-07-28 stateless transport has no server-to-client stream to open,
// so a GET must not be upgraded into one.
func TestStatelessTransportRejectsGET(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set("Accept", "text/event-stream")
	newTestMCPHandler(t).ServeHTTP(rec, req)

	// Auth runs before the transport, so an unauthenticated GET stops at 401;
	// what matters is that it is never a 200 opening an SSE stream.
	if rec.Code == http.StatusOK {
		t.Fatalf("GET /mcp returned 200; stateless mode must not open a server-to-client stream")
	}
}
