package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/version"
)

// SystemHandler handles HTTP requests for system-level endpoints (version, config, health).
type SystemHandler struct {
	cfg *config.Config
	db  *sql.DB
}

// NewSystemHandler creates and returns a new SystemHandler.
// The db parameter is used by the readiness probe; pass nil if not needed.
func NewSystemHandler(cfg *config.Config, db *sql.DB) *SystemHandler {
	return &SystemHandler{cfg: cfg, db: db}
}

// Health godoc
// @Summary      Liveness probe
// @Description  Returns 200 OK with no dependency checks.
// @Tags         infrastructure
// @Produce      json
// @Success      200  {object}  map[string]string
// @Router       /health [get]
func (h *SystemHandler) Health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Ready godoc
// @Summary      Readiness probe
// @Description  Returns 200 when the database is reachable, 503 otherwise.
// @Tags         infrastructure
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /ready [get]
func (h *SystemHandler) Ready(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.db == nil || h.db.PingContext(r.Context()) != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "unavailable", "db": "error"})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "db": "ok"})
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
	w.Header().Set("Content-Type", "application/json")

	versionStr := "Unknown"
	if content, err := os.ReadFile(h.cfg.AllureVersionPath); err == nil {
		versionStr = strings.TrimSpace(string(content))
	}

	_ = json.NewEncoder(w).Encode(VersionResponse{
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
	w.Header().Set("Content-Type", "application/json")

	versionStr := "Unknown"
	if content, err := os.ReadFile(h.cfg.AllureVersionPath); err == nil {
		versionStr = strings.TrimSpace(string(content))
	}

	_ = json.NewEncoder(w).Encode(ConfigResponse{
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
		},
		MetaData: ConfigMetaData{Message: "Config successfully obtained"},
	})
}
