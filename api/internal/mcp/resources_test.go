package mcp_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
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
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// ---------------------------------------------------------------------------
// Test A: small text attachment — content inlined as text
// ---------------------------------------------------------------------------

func TestAttachmentResource_TextInline(t *testing.T) {
	const attachmentID = int64(1)
	const fileContent = "hello from attachment text file"
	const source = "log.txt"
	const storageKey = "proj-storage-key"
	const buildNumber = 7

	mocks := testutil.New()
	mocks.Attachments.GetLocationFn = func(_ context.Context, id int64) (*store.AttachmentLocation, error) {
		if id != attachmentID {
			return nil, store.ErrAttachmentNotFound
		}
		return &store.AttachmentLocation{
			StorageKey:  storageKey,
			BuildNumber: buildNumber,
			Source:      source,
			MimeType:    "text/plain",
			SizeBytes:   int64(len(fileContent)),
		}, nil
	}

	mockStore := &storage.MockStore{
		OpenReportFileFn: func(_ context.Context, projectID, reportID, filePath string) (io.ReadCloser, string, error) {
			wantPath := "data/attachments/" + source
			if projectID == storageKey && reportID == strconv.Itoa(buildNumber) && filePath == wantPath {
				return io.NopCloser(strings.NewReader(fileContent)), "text/plain", nil
			}
			return nil, "", &mockNotFoundError{filePath}
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
	mocks.Attachments.GetLocationFn = func(_ context.Context, id int64) (*store.AttachmentLocation, error) {
		if id != attachmentID {
			return nil, store.ErrAttachmentNotFound
		}
		return &store.AttachmentLocation{
			StorageKey:  "proj-storage-key",
			BuildNumber: 3,
			Source:      "archive.bin",
			MimeType:    "application/octet-stream",
			SizeBytes:   tenMB,
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
// Test C: small image attachment — blob inlined as raw bytes (single-encoded)
// ---------------------------------------------------------------------------

func TestAttachmentResource_ImageBlobSingleEncoded(t *testing.T) {
	const attachmentID = int64(8)
	const source = "shot.png"
	const storageKey = "proj-storage-key"
	const buildNumber = 4
	// Raw PNG-ish bytes including a non-ASCII byte to catch encoding bugs.
	imageBytes := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0xFF, 0x00}

	mocks := testutil.New()
	mocks.Attachments.GetLocationFn = func(_ context.Context, id int64) (*store.AttachmentLocation, error) {
		if id != attachmentID {
			return nil, store.ErrAttachmentNotFound
		}
		return &store.AttachmentLocation{
			StorageKey:  storageKey,
			BuildNumber: buildNumber,
			Source:      source,
			MimeType:    "image/png",
			SizeBytes:   int64(len(imageBytes)),
		}, nil
	}

	mockStore := &storage.MockStore{
		OpenReportFileFn: func(_ context.Context, _, _, _ string) (io.ReadCloser, string, error) {
			return io.NopCloser(bytes.NewReader(imageBytes)), "image/png", nil
		},
	}

	stores := &bootstrap.Stores{Attachment: mocks.Attachments}
	cs := setupResourceServer(t, stores, mockStore, []byte("test-signing-key-32-bytes-padded!"), "http://localhost:8080")

	uri := "alluredeck://attachment/" + strconv.FormatInt(attachmentID, 10)
	res, err := cs.ReadResource(context.Background(), &mcpsdk.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(res.Contents) == 0 {
		t.Fatal("want at least one content item, got none")
	}

	got := res.Contents[0]
	if got.MIMEType != "image/png" {
		t.Errorf("want MIMEType=image/png, got %q", got.MIMEType)
	}
	// The SDK transports Blob as base64 over JSON and decodes it back to raw
	// bytes on the client. A correct (single-encoded) blob round-trips to the
	// original image bytes; a double-encoded blob would arrive as the base64
	// ASCII text of the image bytes instead.
	if !bytes.Equal(got.Blob, imageBytes) {
		t.Errorf("blob mismatch (double-encoding?): want %v, got %v", imageBytes, got.Blob)
	}
	if string(got.Blob) == base64.StdEncoding.EncodeToString(imageBytes) {
		t.Error("blob is double base64-encoded: got the base64 text instead of raw bytes")
	}
}

// ---------------------------------------------------------------------------
// Test D: oversized text attachment — not inlined, signed URL returned instead
// ---------------------------------------------------------------------------

func TestAttachmentResource_TextSizeCap(t *testing.T) {
	const attachmentID = int64(11)
	const source = "huge.log"
	// SizeBytes reports a value above the inline cap → must fall back to URL.
	const hugeSize = int64(5 * 1024 * 1024)

	mocks := testutil.New()
	mocks.Attachments.GetLocationFn = func(_ context.Context, id int64) (*store.AttachmentLocation, error) {
		if id != attachmentID {
			return nil, store.ErrAttachmentNotFound
		}
		return &store.AttachmentLocation{
			StorageKey:  "proj-storage-key",
			BuildNumber: 2,
			Source:      source,
			MimeType:    "text/plain",
			SizeBytes:   hugeSize,
		}, nil
	}

	var openCalled bool
	mockStore := &storage.MockStore{
		OpenReportFileFn: func(_ context.Context, _, _, _ string) (io.ReadCloser, string, error) {
			openCalled = true
			return io.NopCloser(strings.NewReader("should not be read")), "text/plain", nil
		},
	}

	stores := &bootstrap.Stores{Attachment: mocks.Attachments}
	publicURL := "http://localhost:8080"
	cs := setupResourceServer(t, stores, mockStore, []byte("test-signing-key-32-bytes-padded!"), publicURL)

	uri := "alluredeck://attachment/" + strconv.FormatInt(attachmentID, 10)
	res, err := cs.ReadResource(context.Background(), &mcpsdk.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(res.Contents) == 0 {
		t.Fatal("want at least one content item, got none")
	}

	got := res.Contents[0]
	// An oversized text attachment must not be inlined: it should fall back to
	// the signed-download URL, consistent with oversized-image behavior.
	if got.URI == uri {
		t.Fatal("oversized text attachment was inlined; want a signed download URL")
	}
	if !strings.HasPrefix(got.URI, publicURL+"/attachments/") {
		t.Errorf("want signed download URL, got %q", got.URI)
	}
	if openCalled {
		t.Error("storage was read for an oversized text attachment; want no read")
	}
}

// ---------------------------------------------------------------------------
// Test E: path-traversal source — rejected, storage never touched
// ---------------------------------------------------------------------------

func TestAttachmentResource_PathTraversalRejected(t *testing.T) {
	const attachmentID = int64(13)

	traversalSources := []string{
		"../../../etc/passwd",
		"sub/dir/shot.png",
		"..\\..\\windows\\system32",
		"shot.png\x00.txt",
	}

	for _, badSource := range traversalSources {
		t.Run(badSource, func(t *testing.T) {
			mocks := testutil.New()
			mocks.Attachments.GetLocationFn = func(_ context.Context, id int64) (*store.AttachmentLocation, error) {
				return &store.AttachmentLocation{
					StorageKey:  "proj-storage-key",
					BuildNumber: 1,
					Source:      badSource,
					MimeType:    "text/plain",
					SizeBytes:   16,
				}, nil
			}

			var openCalled bool
			mockStore := &storage.MockStore{
				OpenReportFileFn: func(_ context.Context, _, _, _ string) (io.ReadCloser, string, error) {
					openCalled = true
					return io.NopCloser(strings.NewReader("x")), "text/plain", nil
				},
			}

			stores := &bootstrap.Stores{Attachment: mocks.Attachments}
			cs := setupResourceServer(t, stores, mockStore, []byte("test-signing-key-32-bytes-padded!"), "http://localhost:8080")

			uri := "alluredeck://attachment/" + strconv.FormatInt(attachmentID, 10)
			_, err := cs.ReadResource(context.Background(), &mcpsdk.ReadResourceParams{URI: uri})
			if err == nil {
				t.Fatal("want error for path-traversal source, got nil")
			}
			if openCalled {
				t.Error("storage was accessed for a path-traversal source; want no access")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test: attachment not found
// ---------------------------------------------------------------------------

func TestAttachmentResource_NotFound(t *testing.T) {
	mocks := testutil.New()
	// GetLocationFn left nil → returns nil, ErrAttachmentNotFound (mock default)

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
// Test: signed-URL HMAC verification — valid, expired, tampered
// ---------------------------------------------------------------------------

func TestVerifyAttachmentSig(t *testing.T) {
	signingKey := []byte("test-signing-key-32-bytes-padded!")
	const attachmentID = int64(123)
	now := time.Unix(1_700_000_000, 0)

	// Re-derive a valid signature the same way buildSignedURL / signAttachment do:
	// HMAC-SHA256 over "attachment:{id}:exp:{exp}".
	sign := func(id, exp int64) string {
		mac := hmac.New(sha256.New, signingKey)
		mac.Write([]byte("attachment:" + strconv.FormatInt(id, 10) + ":exp:" + strconv.FormatInt(exp, 10)))
		return hex.EncodeToString(mac.Sum(nil))
	}

	futureExp := now.Add(5 * time.Minute).Unix()
	pastExp := now.Add(-time.Minute).Unix()

	tests := []struct {
		name    string
		id      int64
		exp     int64
		sig     string
		wantErr bool
	}{
		{
			name:    "valid signature within expiry",
			id:      attachmentID,
			exp:     futureExp,
			sig:     sign(attachmentID, futureExp),
			wantErr: false,
		},
		{
			name:    "expired URL",
			id:      attachmentID,
			exp:     pastExp,
			sig:     sign(attachmentID, pastExp),
			wantErr: true,
		},
		{
			name:    "tampered signature",
			id:      attachmentID,
			exp:     futureExp,
			sig:     sign(attachmentID, futureExp)[:62] + "ff", // flip trailing hex
			wantErr: true,
		},
		{
			name:    "signature for a different attachment id",
			id:      attachmentID,
			exp:     futureExp,
			sig:     sign(attachmentID+1, futureExp),
			wantErr: true,
		},
		{
			name:    "exp tampered to extend validity",
			id:      attachmentID,
			exp:     futureExp + 3600, // attacker bumps exp but keeps old sig
			sig:     sign(attachmentID, futureExp),
			wantErr: true,
		},
		{
			name:    "missing signature",
			id:      attachmentID,
			exp:     futureExp,
			sig:     "",
			wantErr: true,
		},
		{
			name:    "non-positive exp",
			id:      attachmentID,
			exp:     0,
			sig:     sign(attachmentID, 0),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := internalmcp.VerifyAttachmentSig(signingKey, tc.id, tc.exp, tc.sig, now)
			if tc.wantErr && err == nil {
				t.Fatalf("VerifyAttachmentSig: want error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("VerifyAttachmentSig: want nil error, got %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test: ValidateAttachmentSource — the shared path-traversal guard
// ---------------------------------------------------------------------------

func TestValidateAttachmentSource(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		wantErr bool
	}{
		{"plain filename", "screenshot.png", false},
		{"filename with dashes", "test-result-42.log", false},
		{"filename with single dot", "report.v2.json", false},
		{"empty", "", true},
		{"forward slash", "sub/shot.png", true},
		{"absolute path", "/etc/passwd", true},
		{"backslash", "sub\\shot.png", true},
		{"parent traversal", "../secret", true},
		{"embedded parent traversal", "a/../../secret", true},
		{"dotdot only", "..", true},
		{"NUL byte", "shot.png\x00.txt", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := internalmcp.ValidateAttachmentSource(tc.source)
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateAttachmentSource(%q): want error, got nil", tc.source)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateAttachmentSource(%q): want nil error, got %v", tc.source, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

type mockNotFoundError struct{ path string }

func (e *mockNotFoundError) Error() string { return "not found: " + e.path }
