package mcp_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
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

var resourceSigningKey = []byte("test-signing-key-32-bytes-padded!")

// sigFor derives an attachment download signature independently of the
// production code: HMAC-SHA256 over "attachment:{id}:exp:{exp}".
func sigFor(id, exp int64) string {
	mac := hmac.New(sha256.New, resourceSigningKey)
	mac.Write([]byte("attachment:" + strconv.FormatInt(id, 10) + ":exp:" + strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

// TestAttachmentResource reads alluredeck://attachment/42 through a real MCP
// client session. Small text and raster images are inlined; anything too big,
// non-inlinable, or without a storage backend comes back as a signed download
// URL; a path-traversal source is refused before storage is touched.
func TestAttachmentResource(t *testing.T) {
	const publicURL = "http://localhost:8080"
	const uri = "alluredeck://attachment/42"
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0xFF, 0x00}
	loc := func(source, mime string, size int64) *store.AttachmentLocation {
		return &store.AttachmentLocation{StorageKey: "proj-key", BuildNumber: 7, Source: source, MimeType: mime, SizeBytes: size}
	}

	tests := []struct {
		name       string
		loc        *store.AttachmentLocation // nil: unknown attachment
		noStorage  bool
		body       []byte
		wantText   string
		wantBlob   []byte
		wantSigned bool
		wantErr    bool
	}{
		{name: "small text is inlined from its storage path", loc: loc("log.txt", "text/plain", 31),
			body: []byte("hello from attachment text file"), wantText: "hello from attachment text file"},
		// The SDK base64-encodes Blob on the wire; a double-encoded blob would
		// arrive as the base64 text of the image instead of its bytes.
		{name: "small image is inlined as raw bytes", loc: loc("shot.png", "image/png", int64(len(png))), body: png, wantBlob: png},
		{name: "large binary without storage returns a signed URL", loc: loc("archive.bin", "application/octet-stream", 10<<20),
			noStorage: true, wantSigned: true},
		{name: "oversized text is not read and returns a signed URL", loc: loc("huge.log", "text/plain", 5<<20),
			body: []byte("should not be read"), wantSigned: true},
		{name: "path-traversal source is refused", loc: loc("../../../etc/passwd", "text/plain", 16), body: []byte("x"), wantErr: true},
		{name: "unknown attachment", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mocks := testutil.New()
			mocks.Attachments.GetLocationFn = func(_ context.Context, id int64) (*store.AttachmentLocation, error) {
				if id != 42 || tc.loc == nil {
					return nil, store.ErrAttachmentNotFound
				}
				return tc.loc, nil
			}
			var opened []string
			var dataStore storage.Store
			if !tc.noStorage {
				dataStore = &storage.MockStore{OpenReportFileFn: func(_ context.Context, projectID, reportID, filePath string) (io.ReadCloser, string, error) {
					opened = append(opened, projectID+"|"+reportID+"|"+filePath)
					return io.NopCloser(bytes.NewReader(tc.body)), "", nil
				}}
			}

			srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test-resources", Version: "v0"}, nil)
			internalmcp.RegisterResources(srv, &bootstrap.Stores{Attachment: mocks.Attachments}, zap.NewNop(), resourceSigningKey, publicURL, dataStore)
			st, ct := mcpsdk.NewInMemoryTransports()
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			go srv.Run(ctx, st) //nolint:errcheck
			cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "client", Version: "v0"}, nil).Connect(ctx, ct, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cs.Close() })

			res, err := cs.ReadResource(ctx, &mcpsdk.ReadResourceParams{URI: uri})
			if tc.wantErr {
				if err == nil {
					t.Fatal("want an error, got a resource")
				}
				if len(opened) != 0 {
					t.Errorf("storage was read for a refused attachment: %v", opened)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadResource: %v", err)
			}
			got := res.Contents[0]

			if !tc.wantSigned {
				if want := "proj-key|7|data/attachments/" + tc.loc.Source; len(opened) != 1 || opened[0] != want {
					t.Errorf("storage reads = %v, want [%s]", opened, want)
				}
				if got.URI != uri || got.MIMEType != tc.loc.MimeType || got.Text != tc.wantText || !bytes.Equal(got.Blob, tc.wantBlob) {
					t.Errorf("content = {uri:%q mime:%q text:%q blob:%v}, want {%q %q %q %v}",
						got.URI, got.MIMEType, got.Text, got.Blob, uri, tc.loc.MimeType, tc.wantText, tc.wantBlob)
				}
				return
			}

			if len(opened) != 0 {
				t.Errorf("storage was read for a non-inlined attachment: %v", opened)
			}
			if !strings.HasPrefix(got.URI, publicURL+"/attachments/42?") {
				t.Fatalf("URI = %q, want a signed download URL for attachment 42", got.URI)
			}
			u, err := url.Parse(got.URI)
			if err != nil {
				t.Fatalf("parsing signed URL: %v", err)
			}
			exp, _ := strconv.ParseInt(u.Query().Get("exp"), 10, 64)
			if expT := time.Unix(exp, 0); expT.Before(time.Now().Add(-time.Second)) || expT.After(time.Now().Add(10*time.Minute+time.Second)) {
				t.Errorf("exp = %v, want within 10 minutes from now", expT)
			}
			if sig := u.Query().Get("sig"); sig != sigFor(42, exp) {
				t.Errorf("sig = %q, want HMAC %q", sig, sigFor(42, exp))
			}
		})
	}
}

func TestVerifyAttachmentSig(t *testing.T) {
	const id = int64(123)
	now := time.Unix(1_700_000_000, 0)
	future := now.Add(5 * time.Minute).Unix()
	past := now.Add(-time.Minute).Unix()

	tests := []struct {
		name    string
		id, exp int64
		sig     string
		wantErr bool
	}{
		{"valid signature within expiry", id, future, sigFor(id, future), false},
		{"expired URL", id, past, sigFor(id, past), true},
		{"tampered signature", id, future, sigFor(id, future)[:62] + "ff", true},
		{"signature for a different attachment id", id, future, sigFor(id+1, future), true},
		{"exp bumped to extend validity", id, future + 3600, sigFor(id, future), true},
		{"missing signature", id, future, "", true},
		{"non-positive exp", id, 0, sigFor(id, 0), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := internalmcp.VerifyAttachmentSig(resourceSigningKey, tc.id, tc.exp, tc.sig, now)
			if (err != nil) != tc.wantErr {
				t.Fatalf("VerifyAttachmentSig err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
