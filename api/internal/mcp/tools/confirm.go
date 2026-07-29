package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mkutlak/alluredeck/api/internal/mcp/signed"
)

// confirmInputID names the single input request each propose_* tool issues.
// The client echoes it back as the key in CallToolParams.InputResponses.
const confirmInputID = "confirm"

// confirmTTL bounds how long a pending confirmation stays answerable. Long
// enough for a human to read the prompt and decide, short enough that a
// captured RequestState is not indefinitely replayable.
const confirmTTL = 10 * time.Minute

// writeGate is what evaluateWriteGate decided should happen to a mutating call.
type writeGate int

const (
	// gateWrite: perform the write now.
	gateWrite writeGate = iota
	// gateAsk: return the accompanying result to the client and write nothing.
	gateAsk
	// gateDeclined: the user refused or dismissed the prompt; write nothing.
	gateDeclined
)

// confirmState is the payload sealed into RequestState. It is authenticated,
// not secret — the client already knows every field, having just supplied the
// arguments it summarises.
type confirmState struct {
	// Kind identifies the tool, so a confirmation for one write cannot be
	// replayed against another.
	Kind string `json:"k"`
	// User binds the confirmation to the identity that requested it.
	User string `json:"u"`
	// ArgsHash pins the exact arguments that were shown to the user, so a
	// client cannot get approval for a narrow change and then retry with a
	// broader one.
	ArgsHash string `json:"a"`
}

// clientCanConfirm reports whether the calling client is able to answer a
// confirmation prompt at all.
//
// The gate is elicitation capability, not protocol version. The SDK installs
// serverMultiRoundTripMiddleware by default, which already bridges versions: a
// pre-2026-07-28 client never sees an input_required result, because the
// middleware fulfils the request over the legacy server-initiated elicitation
// call and re-invokes the handler. What that bridge cannot do is serve a client
// with no elicitation support at all — it calls Elicit unconditionally and
// errors out. That is exactly the headless CI case, so such callers must skip
// confirmation and write directly, as they did before this feature existed.
func clientCanConfirm(req *mcpsdk.CallToolRequest) bool {
	if req == nil {
		return false
	}
	caps := req.ClientCapabilities()
	return caps != nil && caps.Elicitation != nil
}

// hashArgs returns a stable fingerprint of a tool's validated arguments.
func hashArgs(args any) (string, error) {
	b, err := json.Marshal(args)
	if err != nil {
		return "", fmt.Errorf("hashing arguments: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// evaluateWriteGate decides whether a mutating tool should write now, ask the
// user first, or abort because the user declined.
//
// It is called after authorisation and argument validation have passed, so the
// prompt describes a write that would genuinely have succeeded.
//
// On the first call it returns (gateAsk, result): result carries the
// confirmation prompt and a signed RequestState, and the caller must return it
// untouched without touching the database. On the retry leg the client echoes
// both back; this function authenticates the state and reports the user's
// answer. Clients that cannot confirm, and deployments with no signing key,
// get gateWrite — preserving the pre-confirmation behaviour rather than
// failing closed on callers that have no way to answer.
func evaluateWriteGate(
	req *mcpsdk.CallToolRequest,
	signingKey []byte,
	kind, userID, prompt string,
	args any,
	now time.Time,
) (writeGate, *mcpsdk.CallToolResult, error) {
	argsHash, err := hashArgs(args)
	if err != nil {
		return gateWrite, nil, err
	}

	// Retry leg: the client has answered a prompt we previously issued.
	if req != nil && req.Params != nil && req.Params.RequestState != "" {
		var state confirmState
		if err := signed.Open(signingKey, req.Params.RequestState, &state, now); err != nil {
			return gateWrite, nil, fmt.Errorf("confirmation state rejected: %w", err)
		}
		if state.Kind != kind {
			return gateWrite, nil, fmt.Errorf("confirmation state is for a different tool")
		}
		if state.User != userID {
			return gateWrite, nil, fmt.Errorf("confirmation state belongs to a different user")
		}
		if state.ArgsHash != argsHash {
			return gateWrite, nil, fmt.Errorf("arguments changed since confirmation was requested")
		}

		resp, ok := req.Params.InputResponses[confirmInputID]
		if !ok {
			return gateWrite, nil, fmt.Errorf("confirmation response missing")
		}
		elicited, ok := resp.(*mcpsdk.ElicitResult)
		if !ok {
			return gateWrite, nil, fmt.Errorf("unexpected confirmation response type %T", resp)
		}
		if elicited.Action != "accept" {
			return gateDeclined, nil, nil
		}
		return gateWrite, nil, nil
	}

	// First leg. A client that cannot answer, or a deployment with no signing
	// key to authenticate the round trip, writes directly as before.
	if len(signingKey) == 0 || !clientCanConfirm(req) {
		return gateWrite, nil, nil
	}

	state, err := signed.Seal(signingKey, confirmState{Kind: kind, User: userID, ArgsHash: argsHash}, confirmTTL, now)
	if err != nil {
		return gateWrite, nil, err
	}

	return gateAsk, &mcpsdk.CallToolResult{
		InputRequests: mcpsdk.InputRequestMap{
			confirmInputID: &mcpsdk.ElicitParams{
				Mode:    "form",
				Message: prompt,
				// No fields to fill in: accept/decline is the whole question,
				// and the client renders that from the action buttons alone.
				RequestedSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
		RequestState: state,
	}, nil
}

// declinedResult reports that a user turned down a proposed write. It is a
// successful call with nothing written, not an error: the user exercising a
// veto is the feature working, and flagging it as an error would invite the
// agent to retry.
func declinedResult(what string) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: "Declined by the user — no " + what + " was created and nothing was written to AllureDeck."},
		},
	}
}
