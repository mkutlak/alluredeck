package pg

// PruneStaleBranchForTest exposes the unexported per-branch prune step so
// tests can drive it directly for a branch the public PruneStaleBranches
// stale-name SELECT would already have filtered out — e.g. a branch promoted
// to default between that SELECT and its per-branch transaction. The in-tx
// is_default re-check is unreachable through the public API in a
// single-goroutine test, so this hook is what pins it.
var PruneStaleBranchForTest = (*BuildStore).pruneStaleBranch
