package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/version"
)

// SystemHandler handles HTTP requests for system-level endpoints (version, config).
type SystemHandler struct {
	cfg *config.Config
}

// NewSystemHandler creates and returns a new SystemHandler.
func NewSystemHandler(cfg *config.Config) *SystemHandler {
	return &SystemHandler{cfg: cfg}
}

// VersionResponse structures

// VersionResponse wraps the version data and metadata for the /version endpoint.
type VersionResponse struct {
	Data     VersionData     `json:"data"`
	MetaData VersionMetaData `json:"meta_data"`
}

// VersionData holds the Allure version string.
type VersionData struct {
	Version string `json:"version"`
}

// VersionMetaData holds the response message for version responses.
type VersionMetaData struct {
	Message string `json:"message"`
}

// Version returns the installed Allure version.
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

// ConfigResponse structures — int fields kept for JSON API compatibility with the
// Python implementation. Booleans are serialised as 0/1 via btoi.

// ConfigResponse wraps the config data and metadata for the /config endpoint.
type ConfigResponse struct {
	Data     ConfigData     `json:"data"`
	MetaData ConfigMetaData `json:"meta_data"`
}

// ConfigData holds the service configuration serialised for API consumers.
type ConfigData struct {
	Version                  string `json:"version"`
	DevMode                  int    `json:"dev_mode"`
	CheckResultsEverySeconds string `json:"check_results_every_seconds"`
	KeepHistory              int    `json:"keep_history"`
	KeepHistoryLatest        int    `json:"keep_history_latest"`
	TLS                      int    `json:"tls"`
	SecurityEnabled          int    `json:"security_enabled"`
	URLPrefix                string `json:"url_prefix"`
	APIRespLessVerbose       int    `json:"api_response_less_verbose"`
	OptimizeStorage          int    `json:"optimize_storage"`
	MakeViewerEndptsPub      int    `json:"make_viewer_endpoints_public"`
	AppVersion               string `json:"app_version"`
	AppBuildDate             string `json:"app_build_date"`
	AppBuildRef              string `json:"app_build_ref"`
}

// ConfigMetaData holds the response message for config responses.
type ConfigMetaData struct {
	Message string `json:"message"`
}

// ConfigEndpoint returns the current service configuration.
func (h *SystemHandler) ConfigEndpoint(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	versionStr := "Unknown"
	if content, err := os.ReadFile(h.cfg.AllureVersionPath); err == nil {
		versionStr = strings.TrimSpace(string(content))
	}

	_ = json.NewEncoder(w).Encode(ConfigResponse{
		Data: ConfigData{
			Version:                  versionStr,
			DevMode:                  btoi(h.cfg.DevMode),
			CheckResultsEverySeconds: h.cfg.CheckResultsSecs,
			KeepHistory:              btoi(h.cfg.KeepHistory),
			KeepHistoryLatest:        h.cfg.KeepHistoryLatest,
			TLS:                      btoi(h.cfg.TLS),
			SecurityEnabled:          btoi(h.cfg.SecurityEnabled),
			URLPrefix:                h.cfg.URLPrefix,
			APIRespLessVerbose:       btoi(h.cfg.APIRespLessVerbose),
			OptimizeStorage:          btoi(h.cfg.OptimizeStorage),
			MakeViewerEndptsPub:      btoi(h.cfg.MakeViewerEndptsPub),
			AppVersion:               version.Version,
			AppBuildDate:             version.BuildDate,
			AppBuildRef:              version.BuildRef,
		},
		MetaData: ConfigMetaData{Message: "Config successfully obtained"},
	})
}

// btoi converts a bool to 0/1 for backward-compatible JSON responses
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
