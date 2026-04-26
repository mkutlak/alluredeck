package handlers

import (
	"context"
	"encoding/json"
	"net"
	"net/http"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/logging"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
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
