// Package tools contains MCP tool implementations for alluredeck.
package tools

import (
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
	RegisterDefectTools(s, stores, logger)
	RegisterKnownIssueTools(s, stores, logger)
	RegisterAttachmentTools(s, stores, logger)
	RegisterMutatingToolsWithURL(s, stores, logger, publicURL, signingKey)
}

// ptr returns a pointer to v. The MCP ToolAnnotations struct models the
// tri-state hints (destructive, open-world) as *bool so that "unset" is
// distinguishable from "false"; we always state them explicitly.
func ptr[T any](v T) *T { return &v }

// readOnlyAnnotations describes every AllureDeck query tool: it only reads,
// returns the same answer for the same arguments, and reaches nothing beyond
// this deployment's own database. Clients use ReadOnlyHint to decide which
// tools are safe to run without prompting the user.
func readOnlyAnnotations() *mcpsdk.ToolAnnotations {
	return &mcpsdk.ToolAnnotations{
		ReadOnlyHint:   true,
		IdempotentHint: true,
		OpenWorldHint:  ptr(false),
	}
}

// proposalAnnotations describes the propose_* tools. They write, so they are
// not read-only, but DestructiveHint is false: each one only ever inserts a
// pending proposal for human review and never updates or deletes existing
// data. They are not idempotent — calling twice queues two proposals.
func proposalAnnotations() *mcpsdk.ToolAnnotations {
	return &mcpsdk.ToolAnnotations{
		ReadOnlyHint:    false,
		DestructiveHint: ptr(false),
		IdempotentHint:  false,
		OpenWorldHint:   ptr(false),
	}
}
