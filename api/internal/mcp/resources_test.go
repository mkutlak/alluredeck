package mcp_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	internalmcp "github.com/mkutlak/alluredeck/api/internal/mcp"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// setupResourceServer wires a minimal MCP server with only resources registered
// (no tools) and returns a connected ClientSession.
func setupResourceServer(
	t *testing.T,
	stores *bootstrap.Stores,
	dataStore storage.Store,
	signingKey []byte,
	publicURL string,
) *mcpsdk.ClientSession {
	t.Helper()
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test-resources", Version: "v0"}, nil)
	internalmcp.RegisterResources(srv, stores, zap.NewNop(), signingKey, publicURL, dataStore)

	st, ct := mcpsdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go srv.Run(ctx, st) //nolint:errcheck

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "client", Version: "v0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// ---------------------------------------------------------------------------
// Test A: small text attachment — content inlined as text
// ---------------------------------------------------------------------------

func TestAttachmentResource_TextInline(t *testing.T) {
	const attachmentID = int64(1)
	const fileContent = "hello from attachment text file"
	const source = "attachments/log.txt"

	mocks := testutil.New()
	mocks.Attachments.GetByIDFn = func(_ context.Context, id int64) (*store.TestAttachment, error) {
		if id != attachmentID {
			return nil, store.ErrAttachmentNotFound
		}
		return &store.TestAttachment{
			ID:        attachmentID,
			Name:      "log.txt",
			Source:    source,
			MimeType:  "text/plain",
			SizeBytes: int64(len(fileContent)),
		}, nil
	}

	mockStore := &storage.MockStore{
		ReadFileFn: func(_ context.Context, _, relPath string) ([]byte, error) {
			if relPath == source {
				return []byte(fileContent), nil
			}
			return nil, &mockNotFoundError{relPath}
		},
	}

	stores := &bootstrap.Stores{Attachment: mocks.Attachments}
	signingKey := []byte("test-signing-key-32-bytes-padded!")
	publicURL := "http://localhost:8080"

	cs := setupResourceServer(t, stores, mockStore, signingKey, publicURL)

	uri := "alluredeck://attachment/" + strconv.FormatInt(attachmentID, 10)
	res, err := cs.ReadResource(context.Background(), &mcpsdk.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(res.Contents) == 0 {
		t.Fatal("want at least one content item, got none")
	}

	got := res.Contents[0]
	if got.URI != uri {
		t.Errorf("want URI=%q, got %q", uri, got.URI)
	}
	if got.MIMEType != "text/plain" {
		t.Errorf("want MIMEType=text/plain, got %q", got.MIMEType)
	}
	if got.Text != fileContent {
		t.Errorf("want text=%q, got %q", fileContent, got.Text)
	}
}

// ---------------------------------------------------------------------------
// Test B: large binary attachment (>2 MB) — signed URL returned, HMAC valid
// ---------------------------------------------------------------------------

func TestAttachmentResource_LargeBinarySignedURL(t *testing.T) {
	const attachmentID = int64(42)
	const tenMB = 10 * 1024 * 1024

	mocks := testutil.New()
	mocks.Attachments.GetByIDFn = func(_ context.Context, id int64) (*store.TestAttachment, error) {
		if id != attachmentID {
			return nil, store.ErrAttachmentNotFound
		}
		return &store.TestAttachment{
			ID:        attachmentID,
			Name:      "archive.bin",
			Source:    "attachments/archive.bin",
			MimeType:  "application/octet-stream",
			SizeBytes: tenMB,
		}, nil
	}

	// dataStore is nil to force the signed-URL path (or could be non-nil — the
	// handler falls back to signed URL for non-text, non-image MIME types).
	stores := &bootstrap.Stores{Attachment: mocks.Attachments}
	signingKey := []byte("test-signing-key-32-bytes-padded!")
	publicURL := "http://localhost:8080"

	cs := setupResourceServer(t, stores, nil, signingKey, publicURL)

	before := time.Now().Add(-time.Second) // allow 1s clock skew
	uri := "alluredeck://attachment/" + strconv.FormatInt(attachmentID, 10)
	res, err := cs.ReadResource(context.Background(), &mcpsdk.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(res.Contents) == 0 {
		t.Fatal("want at least one content item, got none")
	}

	got := res.Contents[0]

	// The returned URI must be a signed download URL, not the original resource URI.
	if got.URI == uri {
		t.Fatal("want a signed download URL as URI, got the original resource URI back")
	}
	if !strings.HasPrefix(got.URI, publicURL+"/attachments/") {
		t.Errorf("want URI to start with %q, got %q", publicURL+"/attachments/", got.URI)
	}

	// Parse the signed URL and verify its HMAC.
	parsed, err := url.Parse(got.URI)
	if err != nil {
		t.Fatalf("parsing signed URL %q: %v", got.URI, err)
	}

	expStr := parsed.Query().Get("exp")
	sig := parsed.Query().Get("sig")
	if expStr == "" || sig == "" {
		t.Fatalf("signed URL missing exp or sig: %q", got.URI)
	}

	expUnix, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		t.Fatalf("parsing exp %q: %v", expStr, err)
	}

	// Expires must be within 10 minutes from now.
	expTime := time.Unix(expUnix, 0)
	after := time.Now().Add(10*time.Minute + time.Second)
	if expTime.Before(before) || expTime.After(after) {
		t.Errorf("exp timestamp %v outside expected window [%v, %v]", expTime, before, after)
	}

	// Verify the HMAC matches.
	payload := "attachment:" + strconv.FormatInt(attachmentID, 10) + ":exp:" + expStr
	mac := hmac.New(sha256.New, signingKey)
	mac.Write([]byte(payload))
	expectedSig := hex.EncodeToString(mac.Sum(nil))
	if sig != expectedSig {
		t.Errorf("HMAC mismatch: want %q, got %q", expectedSig, sig)
	}
}

// ---------------------------------------------------------------------------
// Test: attachment not found
// ---------------------------------------------------------------------------

func TestAttachmentResource_NotFound(t *testing.T) {
	mocks := testutil.New()
	// GetByIDFn left nil → returns nil, ErrAttachmentNotFound (mock default)

	stores := &bootstrap.Stores{Attachment: mocks.Attachments}
	cs := setupResourceServer(t, stores, nil, []byte("key"), "http://localhost")

	_, err := cs.ReadResource(context.Background(), &mcpsdk.ReadResourceParams{
		URI: "alluredeck://attachment/999",
	})
	if err == nil {
		t.Fatal("want error for missing attachment, got nil")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

type mockNotFoundError struct{ path string }

func (e *mockNotFoundError) Error() string { return "not found: " + e.path }
