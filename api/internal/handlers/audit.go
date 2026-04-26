package handlers

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/logging"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// auditFromRequest is a small builder that fills the request-scoped fields of
// an AuditEvent (IP, UserAgent, RequestID) so handlers do not repeat that
// boilerplate at every emit site. Callers populate the action-specific fields
// (Action, Outcome, ActorID/ActorLabel, TargetType/TargetID, Metadata) on the
// returned struct before calling auditRecord.
func auditFromRequest(r *http.Request) store.AuditEvent {
	return store.AuditEvent{
		IP:        clientIPForAudit(r),
		UserAgent: r.UserAgent(),
		RequestID: middleware.RequestIDFromContext(r.Context()),
	}
}

// auditRecord writes evt to logger and never panics. If logger is nil this is
// a no-op — handler tests that do not care about audit wiring can pass nil.
// Recording failures are logged at warn level (so they show up in production
// logs) but never propagated, because audit is best-effort by contract.
func auditRecord(ctx context.Context, logger store.AuditLogger, evt store.AuditEvent) {
	if logger == nil {
		return
	}
	if err := logger.Record(ctx, evt); err != nil {
		logging.FromContext(ctx).Warn("audit: record failed",
			zap.String("action", evt.Action),
			zap.String("outcome", evt.Outcome),
			zap.Error(err),
		)
	}
}

// clientIPForAudit extracts the best-effort client IP from r. We deliberately
// do NOT honour X-Forwarded-For here regardless of cfg.TrustForwardedFor:
// audit logs need to record what the server actually saw at the wire, not
// what the client claimed. The proxy-aware IP, when needed, is already on
// other middleware (rate-limiting); this is a separate concern.
func clientIPForAudit(r *http.Request) string {
	if r.RemoteAddr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// blacklistAccessToken validates tokenStr as expectedType and adds its JTI to
// the JWTManager's persistent blacklist. Used by Logout (refresh+access) and
// by ChangeMyPassword (access). Best-effort: silently returns when jwtMgr is
// nil, the token cannot be parsed, or the claims lack a JTI — callers must
// not depend on this for correctness, only for revocation latency reduction.
//
// defaultExpiry is used when the token's exp claim is missing or unparseable.
// We must use claims.GetExpirationTime() (not a direct type assertion) because
// jwt.Parse stores numeric claims as float64, not *jwt.NumericDate; the
// helper normalises across both shapes.
func blacklistAccessToken(jwtMgr *security.JWTManager, tokenStr, expectedType string, defaultExpiry time.Duration) {
	if jwtMgr == nil || tokenStr == "" {
		return
	}
	_, claims, err := jwtMgr.ValidateToken(tokenStr, expectedType)
	if err != nil {
		return
	}
	jti, ok := claims["jti"].(string)
	if !ok || jti == "" {
		return
	}
	expiry := time.Now().Add(defaultExpiry)
	if expNum, err := claims.GetExpirationTime(); err == nil && expNum != nil {
		expiry = expNum.Time
	}
	jwtMgr.AddToBlacklist(jti, expiry)
}

// auditMetadata is a tiny convenience that marshals a map to json.RawMessage
// and silently returns nil on error. Audit metadata is informational; we never
// fail a request because of a metadata serialisation glitch.
func auditMetadata(fields map[string]any) json.RawMessage {
	if len(fields) == 0 {
		return nil
	}
	b, err := json.Marshal(fields)
	if err != nil {
		return nil
	}
	return b
}
