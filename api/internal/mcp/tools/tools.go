// Package tools contains MCP tool implementations for alluredeck.
package tools

import (
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
)

// RegisterAll registers every MCP tool on the given server.
// Called from api/internal/mcp/server.go's RegisterTools.
//
// publicURL is prepended to the review links returned by the propose_* tools.
// signingKey authenticates the opaque RequestState handed to the client during
// a multi-round-trip write confirmation; both are empty in tests that only
// exercise read tools.
func RegisterAll(s *mcpsdk.Server, stores *bootstrap.Stores, logger *zap.Logger, publicURL string, signingKey []byte) {
	RegisterFailureTools(s, stores, logger)
	RegisterDiscoveryTools(s, stores, logger)
	RegisterHistoryTools(s, stores, logger)
	RegisterDiagnoseTools(s, stores, logger)
	RegisterPipelineTools(s, stores, logger)
	RegisterDefectTools(s, stores, logger)
	RegisterKnownIssueTools(s, stores, logger)
	RegisterAttachmentTools(s, stores, logger)
	RegisterMutatingToolsWithURL(s, stores, logger, publicURL, signingKey)
}

// The MCP ToolAnnotations struct models the tri-state hints (destructive,
// open-world) as *bool so that "unset" is distinguishable from "false". Both
// helpers below state them explicitly rather than leaving them nil, so a client
// never has to guess.

// readOnlyAnnotations describes every AllureDeck query tool: it only reads,
// returns the same answer for the same arguments, and reaches nothing beyond
// this deployment's own database. Clients use ReadOnlyHint to decide which
// tools are safe to run without prompting the user.
func readOnlyAnnotations() *mcpsdk.ToolAnnotations {
	return &mcpsdk.ToolAnnotations{
		ReadOnlyHint:   true,
		IdempotentHint: true,
		OpenWorldHint:  new(false),
	}
}

// digestMaxLen bounds the unstructured text digest a read tool returns. The
// digest is a headline, not a second copy of the payload; anything longer is
// truncated rather than allowed to grow with the data.
const digestMaxLen = 200

// textResult returns a CallToolResult carrying digest as its only unstructured
// content.
//
// The SDK fills CallToolResult.Content with the JSON serialization of the whole
// structured output whenever a handler leaves it nil, so a tool that returns
// nil ships its entire payload twice — once as structuredContent and once as
// text — and the client pays tokens for both. Returning an explicit one-line
// digest keeps the text channel to a headline while structuredContent still
// carries the full result.
func textResult(digest string) *mcpsdk.CallToolResult {
	if len(digest) > digestMaxLen {
		// Cut on bytes, then drop any partial rune the cut created.
		digest = strings.ToValidUTF8(digest[:digestMaxLen-3], "") + "..."
	}
	return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: digest}}}
}

// proposalAnnotations describes the propose_* tools. They write, so they are
// not read-only, but DestructiveHint is false: each one only ever inserts a
// pending proposal for human review and never updates or deletes existing
// data. They are not idempotent — calling twice queues two proposals.
func proposalAnnotations() *mcpsdk.ToolAnnotations {
	return &mcpsdk.ToolAnnotations{
		ReadOnlyHint:    false,
		DestructiveHint: new(false),
		IdempotentHint:  false,
		OpenWorldHint:   new(false),
	}
}
