package mcp

import (
	"strings"
	"testing"
)

// TestServerInstructions_DedupeAndAttachmentRules verifies the two workflow
// rules added to serverInstructions: full_name-based dedup correlation (legacy
// builds double-ingest a test under two history_id schemes) and preferring the
// get_attachment tool over the alluredeck:// resource (some MCP gateways proxy
// tools but not resources).
func TestServerInstructions_DedupeAndAttachmentRules(t *testing.T) {
	if !strings.Contains(serverInstructions, "merged_history_ids") {
		t.Error("want serverInstructions to mention merged_history_ids")
	}
	if !strings.Contains(serverInstructions, "key on full_name, never history_id") {
		t.Error("want serverInstructions to instruct keying on full_name, never history_id")
	}
	if !strings.Contains(serverInstructions, "get_attachment TOOL") {
		t.Error("want serverInstructions to instruct using the get_attachment tool")
	}
	if !strings.Contains(serverInstructions, "alluredeck://attachment/{id} resource") {
		t.Error("want serverInstructions to name the alluredeck:// attachment resource as the fallback being warned about")
	}
}
