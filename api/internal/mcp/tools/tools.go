// Package tools contains MCP tool implementations for alluredeck.
package tools

import (
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
)

// RegisterAll registers every MCP tool on the given server.
// Called from api/internal/mcp/server.go's RegisterTools.
func RegisterAll(s *mcpsdk.Server, stores *bootstrap.Stores, logger *zap.Logger) {
	RegisterFailureTools(s, stores, logger)
	RegisterDiscoveryTools(s, stores, logger)
	RegisterHistoryTools(s, stores, logger)
	RegisterDefectTools(s, stores, logger)
	RegisterKnownIssueTools(s, stores, logger)
	RegisterAttachmentTools(s, stores, logger)
	RegisterMutatingTools(s, stores, logger)
}
