package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// clusterNoMessageKey stands in for an absent error message in the cluster key,
// so tests with no message group together instead of each forming a cluster of
// one keyed on the empty string. It is a key only — it never reaches the
// output.
const clusterNoMessageKey = "(no error message)"

// clusterKeyLen is how many hex chars of the digest ClusterKey keeps: 64 bits
// is ample for telling the clusters of one run apart, and short enough to ship
// on every failure.
const clusterKeyLen = 16

// ClusterKey is the failure shape two tests must share to be one root cause:
// the same error message once dynamic values are normalized away, the same
// lifecycle phase, and the same deepest failed step. The deepest step alone is
// used rather than the whole path because the outer steps differ per test while
// the step that actually blew up does not.
//
// diagnose_pipeline and the REST failures list both build the key from this
// function (and ClusterMessage for its message), so the IDE and the browser
// group the same tests together. diagnose_failure shares the function but not
// the inputs: it passes the deepest step's message alone, uncapped.
// The shape is hashed so the key is short and free of the NUL separators.
func ClusterKey(message, phase, lastStep string) string {
	msg := NormalizeMessage(strings.TrimSpace(message))
	if strings.TrimSpace(msg) == "" {
		msg = clusterNoMessageKey
	}
	sum := sha256.Sum256([]byte(msg + "\x00" + phase + "\x00" + lastStep))
	return hex.EncodeToString(sum[:])[:clusterKeyLen]
}

// clusterMessageMax is how many BYTES of a failure message are clustered on.
// It is a byte count, not a character count, so a message in any script is cut
// at the same place wherever it was read.
const clusterMessageMax = 500

// ClusterMessage picks the message a failing test is clustered on: the deepest
// failed step's, which says what actually blew up, else the test's own, cut to
// clusterMessageMax bytes. The cut may fall inside a multi-byte rune; that is
// harmless to the key and keeps the text identical wherever it is computed.
//
// It is shared by diagnose_pipeline and the REST failures list. ClusterKey does
// not cut by itself: diagnose_failure passes it uncapped messages.
func ClusterMessage(stepMessage, testMessage string) string {
	msg := testMessage
	if stepMessage != "" {
		msg = stepMessage
	}
	if len(msg) > clusterMessageMax {
		msg = msg[:clusterMessageMax]
	}
	return msg
}
