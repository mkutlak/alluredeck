package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// S3Config holds S3/MinIO connection settings.
type S3Config struct {
	Endpoint  string // e.g. "http://minio:9000" or "https://s3.amazonaws.com"
	Bucket    string
	Region    string
	AccessKey string //nolint:gosec // G117: field name matches secret pattern; this is intentional
	SecretKey string
	UseSSL    bool
	PathStyle bool // true for MinIO (path-style URLs)
}

// Config holds the application configuration loaded from environment variables
type Config struct {
	Port                string
	DevMode             bool
	SecurityEnabled     bool
	SecurityUser        string
	SecurityPass        string
	ViewerUser          string
	ViewerPass          string
	JWTSecret           string //nolint:gosec // field name is intentional, not a leaked secret
	MakeViewerEndptsPub bool
	AllureVersionPath   string
	StaticContentPath   string
	ProjectsDirectory   string
	CheckResultsSecs    string // "NONE" or positive integer string
	KeepHistory         bool
	KeepHistoryLatest   int
	TLS                 bool
	URLPrefix           string
	APIRespLessVerbose  bool
	OptimizeStorage     bool // dead in Go runtime; kept for /config response compatibility
	LegacyUI            bool
	CORSAllowedOrigins  []string
	AccessTokenExpiry   time.Duration
	RefreshTokenExpiry  time.Duration
	DatabasePath        string
	StorageType         string // "local" (default) or "s3"
	S3                  S3Config
}

// yamlConfig is the intermediate representation for YAML parsing.
// Pointer fields distinguish "not set" (nil) from "zero value".
type yamlConfig struct {
	Port                *string  `yaml:"port"`
	DevMode             *bool    `yaml:"dev_mode"`
	SecurityEnabled     *bool    `yaml:"security_enabled"`
	SecurityUser        *string  `yaml:"security_user"`
	SecurityPass        *string  `yaml:"security_pass"`
	ViewerUser          *string  `yaml:"viewer_user"`
	ViewerPass          *string  `yaml:"viewer_pass"`
	JWTSecret           *string  `yaml:"jwt_secret"` //nolint:gosec // field name is intentional, not a leaked secret
	MakeViewerEndptsPub *bool    `yaml:"make_viewer_endpoints_public"`
	AllureVersionPath   *string  `yaml:"allure_version_path"`
	StaticContentPath   *string  `yaml:"static_content_path"`
	ProjectsDirectory   *string  `yaml:"projects_directory"`
	CheckResultsSecs    *string  `yaml:"check_results_secs"`
	KeepHistory         *bool    `yaml:"keep_history"`
	KeepHistoryLatest   *int     `yaml:"keep_history_latest"`
	TLS                 *bool    `yaml:"tls"`
	URLPrefix           *string  `yaml:"url_prefix"`
	APIRespLessVerbose  *bool    `yaml:"api_response_less_verbose"`
	OptimizeStorage     *bool    `yaml:"optimize_storage"`
	LegacyUI            *bool    `yaml:"legacy_ui"`
	CORSAllowedOrigins  []string `yaml:"cors_allowed_origins"`
	AccessTokenExpSecs  *int     `yaml:"access_token_expiry_secs"`
	RefreshTokenExpSecs *int     `yaml:"refresh_token_expiry_secs"`
	DatabasePath        *string  `yaml:"database_path"`
	StorageType         *string  `yaml:"storage_type"`
	S3Endpoint          *string  `yaml:"s3_endpoint"`
	S3Bucket            *string  `yaml:"s3_bucket"`
	S3Region            *string  `yaml:"s3_region"`
	S3AccessKey         *string  `yaml:"s3_access_key"`
	S3SecretKey         *string  `yaml:"s3_secret_key"`
	S3UseSSL            *bool    `yaml:"s3_use_ssl"`
	S3PathStyle         *bool    `yaml:"s3_path_style"`
}

const defaultJWTSecret = "super-secret-key-for-dev"

// LoadConfig parses environment variables (with optional YAML file fallback) and returns a populated Config struct.
// Precedence (highest to lowest): env vars > YAML file > hardcoded defaults.
// The YAML file path is read from CONFIG_FILE env var (default: /app/config.yaml).
// A missing config file is silently ignored; a malformed file causes a fatal log.
func LoadConfig() *Config {
	configFile := getEnv("CONFIG_FILE", "/app/config.yaml")
	yc, err := loadFromYAML(configFile)
	if err != nil {
		log.Fatalf("ERROR: failed to parse config file %q: %v", configFile, err)
	}
	if yc == nil {
		yc = &yamlConfig{}
	}

	jwtSecret := getEnvOrYAML("JWT_SECRET_KEY", yc.JWTSecret, defaultJWTSecret)
	securityEnabled := getEnvOrYAMLBool("SECURITY_ENABLED", yc.SecurityEnabled)

	// CORS: env var (comma-separated string) takes priority over YAML list.
	var corsOrigins []string
	if corsEnv, exists := os.LookupEnv("CORS_ALLOWED_ORIGINS"); exists {
		corsOrigins = parseCORSOrigins(corsEnv)
	} else if len(yc.CORSAllowedOrigins) > 0 {
		corsOrigins = yc.CORSAllowedOrigins
	}

	return &Config{
		Port:                getEnvOrYAML("PORT", yc.Port, "5050"),
		DevMode:             getEnvOrYAMLBool("DEV_MODE", yc.DevMode),
		SecurityEnabled:     securityEnabled,
		SecurityUser:        getEnvOrYAML("SECURITY_USER", yc.SecurityUser, ""),
		SecurityPass:        getEnvOrYAML("SECURITY_PASS", yc.SecurityPass, ""),
		ViewerUser:          getEnvOrYAML("SECURITY_VIEWER_USER", yc.ViewerUser, ""),
		ViewerPass:          getEnvOrYAML("SECURITY_VIEWER_PASS", yc.ViewerPass, ""),
		JWTSecret:           jwtSecret,
		MakeViewerEndptsPub: getEnvOrYAMLBool("MAKE_VIEWER_ENDPOINTS_PUBLIC", yc.MakeViewerEndptsPub),
		AllureVersionPath:   getEnvOrYAML("ALLURE_VERSION", yc.AllureVersionPath, "/version"),
		StaticContentPath:   getEnvOrYAML("STATIC_CONTENT", yc.StaticContentPath, "/app/static"),
		ProjectsDirectory:   getEnvOrYAML("STATIC_CONTENT_PROJECTS", yc.ProjectsDirectory, "/app/projects"),
		CheckResultsSecs:    getEnvOrYAML("CHECK_RESULTS_EVERY_SECONDS", yc.CheckResultsSecs, "1"),
		KeepHistory:         getEnvOrYAMLBool("KEEP_HISTORY", yc.KeepHistory),
		KeepHistoryLatest:   getEnvOrYAMLInt("KEEP_HISTORY_LATEST", yc.KeepHistoryLatest, 20),
		TLS:                 getEnvOrYAMLBool("TLS", yc.TLS),
		URLPrefix:           getEnvOrYAML("URL_PREFIX", yc.URLPrefix, ""),
		APIRespLessVerbose:  getEnvOrYAMLBool("API_RESPONSE_LESS_VERBOSE", yc.APIRespLessVerbose),
		OptimizeStorage:     getEnvOrYAMLBool("OPTIMIZE_STORAGE", yc.OptimizeStorage),
		LegacyUI:            getEnvOrYAMLBool("LEGACY_UI", yc.LegacyUI),
		CORSAllowedOrigins:  corsOrigins,
		AccessTokenExpiry:   time.Duration(getEnvOrYAMLInt("JWT_ACCESS_TOKEN_EXPIRES", yc.AccessTokenExpSecs, 900)) * time.Second,
		RefreshTokenExpiry:  time.Duration(getEnvOrYAMLInt("JWT_REFRESH_TOKEN_EXPIRES", yc.RefreshTokenExpSecs, 2592000)) * time.Second,
		DatabasePath:        getEnvOrYAML("DATABASE_PATH", yc.DatabasePath, "/app/allure.db"),
		StorageType:         getEnvOrYAML("STORAGE_TYPE", yc.StorageType, "local"),
		S3: S3Config{
			Endpoint:  getEnvOrYAML("S3_ENDPOINT", yc.S3Endpoint, ""),
			Bucket:    getEnvOrYAML("S3_BUCKET", yc.S3Bucket, ""),
			Region:    getEnvOrYAML("S3_REGION", yc.S3Region, "us-east-1"),
			AccessKey: getEnvOrYAML("S3_ACCESS_KEY", yc.S3AccessKey, ""),
			SecretKey: getEnvOrYAML("S3_SECRET_KEY", yc.S3SecretKey, ""),
			UseSSL:    getEnvOrYAMLBool("S3_USE_SSL", yc.S3UseSSL),
			PathStyle: getEnvOrYAMLBool("S3_PATH_STYLE", yc.S3PathStyle),
		},
	}
}

// ErrInsecureJWTSecret is returned when security is enabled with the default secret.
var ErrInsecureJWTSecret = errors.New("SECURITY_ENABLED is true but JWT_SECRET_KEY is the insecure default; set JWT_SECRET_KEY to a strong random secret")

// ErrS3EndpointRequired is returned when storage_type is "s3" but S3_ENDPOINT is not set.
var ErrS3EndpointRequired = errors.New("S3_ENDPOINT is required when STORAGE_TYPE=s3")

// ErrS3BucketRequired is returned when storage_type is "s3" but S3_BUCKET is not set.
var ErrS3BucketRequired = errors.New("S3_BUCKET is required when STORAGE_TYPE=s3")

// Validate checks that the configuration is safe to run in production.
// Returns an error if security is enabled but the insecure default JWT secret is used,
// or if S3 storage is selected but required fields are missing.
func (c *Config) Validate() error {
	if c.SecurityEnabled && c.JWTSecret == defaultJWTSecret {
		return ErrInsecureJWTSecret
	}
	if c.StorageType == "s3" {
		if c.S3.Endpoint == "" {
			return ErrS3EndpointRequired
		}
		if c.S3.Bucket == "" {
			return ErrS3BucketRequired
		}
	}
	return nil
}

// loadFromYAML reads the YAML file at path and returns a parsed yamlConfig.
// Returns (nil, nil) if the file does not exist.
// Returns (nil, error) if the file exists but cannot be read or parsed.
func loadFromYAML(path string) (*yamlConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil //nolint:nilnil // intentional: nil means "no config file", not an error
		}
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}
	var yc yamlConfig
	if err := yaml.Unmarshal(data, &yc); err != nil {
		return nil, fmt.Errorf("parse config file %q: %w", path, err)
	}
	return &yc, nil
}

func parseCORSOrigins(raw string) []string {
	var origins []string
	for o := range strings.SplitSeq(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}

// getEnvOrYAML returns the env var value if set, else the YAML pointer value if non-nil, else defaultValue.
func getEnvOrYAML(envKey string, yamlVal *string, defaultValue string) string {
	if val, exists := os.LookupEnv(envKey); exists {
		return val
	}
	if yamlVal != nil {
		return *yamlVal
	}
	return defaultValue
}

// getEnvOrYAMLBool returns the env var value if set, else the YAML pointer value if non-nil, else false.
func getEnvOrYAMLBool(envKey string, yamlVal *bool) bool {
	if valStr, exists := os.LookupEnv(envKey); exists {
		switch strings.ToUpper(valStr) {
		case "1", "TRUE", "YES":
			return true
		case "0", "FALSE", "NO":
			return false
		default:
			log.Printf("WARNING: invalid boolean value for %s: %q, using default false", envKey, valStr)
		}
	}
	if yamlVal != nil {
		return *yamlVal
	}
	return false
}

// getEnvOrYAMLInt returns the env var value if set, else the YAML pointer value if non-nil, else defaultValue.
func getEnvOrYAMLInt(envKey string, yamlVal *int, defaultValue int) int {
	if valStr, exists := os.LookupEnv(envKey); exists {
		if i, err := strconv.Atoi(valStr); err == nil {
			return i
		}
		log.Printf("WARNING: invalid integer value for %s: %q, using default %d", envKey, valStr, defaultValue)
	}
	if yamlVal != nil {
		return *yamlVal
	}
	return defaultValue
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valStr := getEnv(key, "")
	if valStr != "" {
		if i, err := strconv.Atoi(valStr); err == nil {
			return i
		}
		log.Printf("WARNING: invalid integer value for %s: %q, using default %d", key, valStr, defaultValue)
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	valStr := getEnv(key, "")
	if valStr != "" {
		switch strings.ToUpper(valStr) {
		case "1", "TRUE", "YES":
			return true
		case "0", "FALSE", "NO":
			return false
		default:
			log.Printf("WARNING: invalid boolean value for %s: %q, using default %v", key, valStr, defaultValue)
		}
	}
	return defaultValue
}
