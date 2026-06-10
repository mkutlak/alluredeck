package handlers

import (
	"database/sql"
	"net/http"
	"os"
	"strings"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/version"
)

// SystemHandler handles HTTP requests for system-level endpoints (version, config, health).
type SystemHandler struct {
	cfg       *config.Config
	db        *sql.DB
	dataStore storage.Store
	queue     runner.JobQueuer
}

// NewSystemHandler creates and returns a new SystemHandler. The db, dataStore,
// and queue dependencies are probed by the readiness endpoint; pass nil for any
// that should be skipped (a nil dependency never fails readiness).
func NewSystemHandler(cfg *config.Config, db *sql.DB, dataStore storage.Store, queue runner.JobQueuer) *SystemHandler {
	return &SystemHandler{cfg: cfg, db: db, dataStore: dataStore, queue: queue}
}

// Health godoc
// @Summary      Liveness probe
// @Description  Returns 200 OK with no dependency checks.
// @Tags         infrastructure
// @Produce      json
// @Success      200  {object}  map[string]string
// @Router       /health [get]
func (h *SystemHandler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready godoc
// @Summary      Readiness probe
// @Description  Probes each backing dependency (database, storage backend, job
// @Description  queue) and returns 200 only when all wired dependencies are
// @Description  healthy, 503 otherwise. The JSON body reports per-dependency
// @Description  status ("ok"/"error"/"skipped") so an operator can see which
// @Description  dependency is degraded. A nil (unwired) dependency is "skipped"
// @Description  and never fails readiness.
// @Tags         infrastructure
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /ready [get]
func (h *SystemHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	statuses := map[string]string{}
	healthy := true

	check := func(name string, present bool, probe func() error) {
		switch {
		case !present:
			statuses[name] = "skipped"
		case probe() != nil:
			statuses[name] = "error"
			healthy = false
		default:
			statuses[name] = "ok"
		}
	}

	check("db", h.db != nil, func() error { return h.db.PingContext(ctx) })
	check("storage", h.dataStore != nil, func() error { return h.dataStore.HealthCheck(ctx) })
	check("queue", h.queue != nil, func() error { return h.queue.Healthy(ctx) })

	if healthy {
		statuses["status"] = "ok"
		writeJSON(w, http.StatusOK, statuses)
		return
	}
	statuses["status"] = "unavailable"
	writeJSON(w, http.StatusServiceUnavailable, statuses)
}

// VersionResponse structures

// VersionResponse wraps the version data and metadata for the /version endpoint.
type VersionResponse struct {
	Data     VersionData     `json:"data"`
	MetaData VersionMetaData `json:"metadata"`
}

// VersionData holds the Allure version string.
type VersionData struct {
	Version string `json:"version"`
}

// VersionMetaData holds the response message for version responses.
type VersionMetaData struct {
	Message string `json:"message"`
}

// Version godoc
// @Summary      Get Allure version
// @Description  Returns the installed Allure CLI version.
// @Tags         system
// @Produce      json
// @Success      200  {object}  VersionResponse
// @Router       /version [get]
func (h *SystemHandler) Version(w http.ResponseWriter, _ *http.Request) {
	versionStr := "Unknown"
	if content, err := os.ReadFile(h.cfg.AllureVersionPath); err == nil {
		versionStr = strings.TrimSpace(string(content))
	}

	writeJSON(w, http.StatusOK, VersionResponse{
		Data:     VersionData{Version: versionStr},
		MetaData: VersionMetaData{Message: "Version successfully obtained"},
	})
}

// ConfigResponse wraps the config data and metadata for the /config endpoint.
type ConfigResponse struct {
	Data     ConfigData     `json:"data"`
	MetaData ConfigMetaData `json:"metadata"`
}

// ConfigData holds the service configuration serialised for API consumers.
type ConfigData struct {
	Version                   string `json:"version"`
	DevMode                   bool   `json:"dev_mode"`
	CheckResultsEverySeconds  string `json:"check_results_every_seconds"`
	KeepHistory               bool   `json:"keep_history"`
	KeepHistoryLatest         int    `json:"keep_history_latest"`
	TLS                       bool   `json:"tls"`
	SecurityEnabled           bool   `json:"security_enabled"`
	APIResponseLessVerbose    bool   `json:"api_response_less_verbose"`
	MakeViewerEndpointsPublic bool   `json:"make_viewer_endpoints_public"`
	AppVersion                string `json:"app_version"`
	AppBuildDate              string `json:"app_build_date"`
	AppBuildRef               string `json:"app_build_ref"`
	OIDCEnabled               bool   `json:"oidc_enabled"`
	MCPEnabled                bool   `json:"mcp_enabled"`
}

// ConfigMetaData holds the response message for config responses.
type ConfigMetaData struct {
	Message string `json:"message"`
}

// ConfigEndpoint godoc
// @Summary      Get service configuration
// @Description  Returns the current AllureDeck service configuration.
// @Tags         system
// @Produce      json
// @Success      200  {object}  ConfigResponse
// @Router       /config [get]
func (h *SystemHandler) ConfigEndpoint(w http.ResponseWriter, _ *http.Request) {
	versionStr := "Unknown"
	if content, err := os.ReadFile(h.cfg.AllureVersionPath); err == nil {
		versionStr = strings.TrimSpace(string(content))
	}

	writeJSON(w, http.StatusOK, ConfigResponse{
		Data: ConfigData{
			Version:                   versionStr,
			DevMode:                   h.cfg.DevMode,
			CheckResultsEverySeconds:  h.cfg.CheckResultsEverySeconds,
			KeepHistory:               h.cfg.KeepHistory,
			KeepHistoryLatest:         h.cfg.KeepHistoryLatest,
			TLS:                       h.cfg.TLS,
			SecurityEnabled:           h.cfg.SecurityEnabled,
			APIResponseLessVerbose:    h.cfg.APIResponseLessVerbose,
			MakeViewerEndpointsPublic: h.cfg.MakeViewerEndpointsPublic,
			AppVersion:                version.Version,
			AppBuildDate:              version.BuildDate,
			AppBuildRef:               version.BuildRef,
			OIDCEnabled:               h.cfg.OIDC.Enabled,
			MCPEnabled:                h.cfg.MCPServerEnabled,
		},
		MetaData: ConfigMetaData{Message: "Config successfully obtained"},
	})
}
