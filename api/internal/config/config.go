package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
	"go.yaml.in/yaml/v3"
	"golang.org/x/crypto/bcrypt"
)

// DurationSeconds accepts both integer seconds (900) and Go duration strings ("15m").
// It implements envconfig.Decoder and yaml.v3 Unmarshaler for seamless configuration loading.
type DurationSeconds time.Duration

// Decode implements the envconfig.Decoder interface.
func (d *DurationSeconds) Decode(value string) error {
	if secs, err := strconv.Atoi(value); err == nil {
		*d = DurationSeconds(time.Duration(secs) * time.Second)
		return nil
	}
	dur, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: must be integer seconds or Go duration (e.g. 15m)", value)
	}
	*d = DurationSeconds(dur)
	return nil
}

// UnmarshalYAML implements the yaml.v3 Unmarshaler interface.
func (d *DurationSeconds) UnmarshalYAML(value *yaml.Node) error {
	var secs int
	if err := value.Decode(&secs); err == nil {
		*d = DurationSeconds(time.Duration(secs) * time.Second)
		return nil
	}
	var s string
	if err := value.Decode(&s); err != nil {
		return fmt.Errorf("duration must be integer seconds or string: %w", err)
	}
	return d.Decode(s)
}

// Duration returns the underlying time.Duration value.
func (d DurationSeconds) Duration() time.Duration {
	return time.Duration(d)
}

// Seconds returns the duration as seconds (float64).
func (d DurationSeconds) Seconds() float64 {
	return time.Duration(d).Seconds()
}

// S3Config holds S3/MinIO connection settings.
type S3Config struct {
	Endpoint              string `yaml:"endpoint" envconfig:"ENDPOINT"`
	Bucket                string `yaml:"bucket" envconfig:"BUCKET"`
	Region                string `yaml:"region" envconfig:"REGION"`
	AccessKey             string `yaml:"access_key" envconfig:"ACCESS_KEY"` //nolint:gosec // G117: field name matches secret pattern; this is intentional
	SecretKey             string `yaml:"secret_key" envconfig:"SECRET_KEY"`
	TLSInsecureSkipVerify bool   `yaml:"tls_insecureskipverify" envconfig:"TLS_INSECURESKIPVERIFY"`
	PathStyle             bool   `yaml:"path_style" envconfig:"PATH_STYLE"`
	Concurrency           int    `yaml:"concurrency" envconfig:"CONCURRENCY"`
}

// OIDCConfig holds OpenID Connect / SSO configuration.
type OIDCConfig struct {
	Enabled           bool     `yaml:"enabled" envconfig:"OIDC_ENABLED"`
	IssuerURL         string   `yaml:"issuer_url" envconfig:"OIDC_ISSUER_URL"`
	ClientID          string   `yaml:"client_id" envconfig:"OIDC_CLIENT_ID"`
	ClientSecret      string   `yaml:"client_secret" envconfig:"OIDC_CLIENT_SECRET"` //nolint:gosec // field name is intentional, not a leaked secret
	RedirectURL       string   `yaml:"redirect_url" envconfig:"OIDC_REDIRECT_URL"`
	Scopes            []string `yaml:"scopes" envconfig:"OIDC_SCOPES"`
	GroupsClaim       string   `yaml:"groups_claim" envconfig:"OIDC_GROUPS_CLAIM"`
	AdminGroups       []string `yaml:"admin_groups" envconfig:"OIDC_ADMIN_GROUPS"`
	EditorGroups      []string `yaml:"editor_groups" envconfig:"OIDC_EDITOR_GROUPS"`
	DefaultRole       string   `yaml:"default_role" envconfig:"OIDC_DEFAULT_ROLE"`
	StateCookieSecret string   `yaml:"state_cookie_secret" envconfig:"OIDC_STATE_COOKIE_SECRET"` //nolint:gosec // field name is intentional
	PostLoginRedirect string   `yaml:"post_login_redirect" envconfig:"OIDC_POST_LOGIN_REDIRECT"`
	EndSessionURL     string   `yaml:"end_session_url" envconfig:"OIDC_END_SESSION_URL"`
}

// Config holds the application configuration loaded from environment variables
// and an optional YAML configuration file.
// Precedence (highest to lowest): env vars > YAML file > hardcoded defaults.
type Config struct {
	Port                      string          `yaml:"port" envconfig:"PORT"`
	DevMode                   bool            `yaml:"dev_mode" envconfig:"DEV_MODE"`
	SecurityEnabled           bool            `yaml:"security_enabled" envconfig:"SECURITY_ENABLED"`
	AdminUser                 string          `yaml:"admin_user" envconfig:"ADMIN_USER"`
	AdminPass                 string          `yaml:"admin_pass" envconfig:"ADMIN_PASS"`
	ViewerUser                string          `yaml:"viewer_user" envconfig:"VIEWER_USER"`
	ViewerPass                string          `yaml:"viewer_pass" envconfig:"VIEWER_PASS"`
	JWTSecret                 string          `yaml:"jwt_secret_key" envconfig:"JWT_SECRET_KEY"` //nolint:gosec // field name is intentional, not a leaked secret
	MakeViewerEndpointsPublic bool            `yaml:"make_viewer_endpoints_public" envconfig:"MAKE_VIEWER_ENDPOINTS_PUBLIC"`
	AllureVersionPath         string          `yaml:"allure_version_path" envconfig:"ALLURE_VERSION_FILE"`
	ProjectsPath              string          `yaml:"projects_path" envconfig:"PROJECTS_PATH"`
	CheckResultsEverySeconds  string          `yaml:"check_results_every_secs" envconfig:"CHECK_RESULTS_EVERY_SECONDS"`
	KeepHistory               bool            `yaml:"keep_history" envconfig:"KEEP_HISTORY"`
	KeepHistoryLatest         int             `yaml:"keep_history_latest" envconfig:"KEEP_HISTORY_LATEST"`
	KeepHistoryMaxAgeDays     int             `yaml:"keep_history_max_age_days" envconfig:"KEEP_HISTORY_MAX_AGE_DAYS"`
	PendingResultsMaxAgeDays  int             `yaml:"pending_results_max_age_days" envconfig:"PENDING_RESULTS_MAX_AGE_DAYS"`
	TLS                       bool            `yaml:"tls" envconfig:"TLS"`
	APIResponseLessVerbose    bool            `yaml:"api_response_less_verbose" envconfig:"API_RESPONSE_LESS_VERBOSE"`
	CORSAllowedOrigins        []string        `yaml:"cors_allowed_origins" envconfig:"CORS_ALLOWED_ORIGINS"`
	TrustForwardedFor         bool            `yaml:"trust_forwarded_for" envconfig:"TRUST_FORWARDED_FOR"`
	SwaggerEnabled            bool            `yaml:"swagger_enabled" envconfig:"SWAGGER_ENABLED"`
	SwaggerHost               string          `yaml:"swagger_host" envconfig:"SWAGGER_HOST"`
	AccessTokenExpiry         DurationSeconds `yaml:"jwt_access_token_expires" envconfig:"JWT_ACCESS_TOKEN_EXPIRES"`
	RefreshTokenExpiry        DurationSeconds `yaml:"jwt_refresh_token_expires" envconfig:"JWT_REFRESH_TOKEN_EXPIRES"`
	ReportGenerationTimeout   DurationSeconds `yaml:"report_generation_timeout" envconfig:"REPORT_GENERATION_TIMEOUT"`
	// PostgreSQL connection URL
	DatabaseURL string `yaml:"database_url" envconfig:"DATABASE_URL"`
	// PostgreSQL connection pool settings
	DBMaxOpenConns    int           `yaml:"db_max_open_conns" envconfig:"DB_MAX_OPEN_CONNS"`
	DBMaxIdleConns    int           `yaml:"db_max_idle_conns" envconfig:"DB_MAX_IDLE_CONNS"`
	DBConnMaxLifetime time.Duration `yaml:"db_conn_max_lifetime" envconfig:"DB_CONN_MAX_LIFETIME"`
	StorageType       string        `yaml:"storage_type" envconfig:"STORAGE_TYPE"`
	S3                S3Config      `yaml:"s3"`
	OIDC              OIDCConfig    `yaml:"oidc"`
	LogLevel          string        `yaml:"log_level" envconfig:"LOG_LEVEL"`
	MaxUploadSizeMB   int           `yaml:"max_upload_size_mb" envconfig:"MAX_UPLOAD_SIZE_MB"`
	ExternalURL       string        `yaml:"external_url" envconfig:"EXTERNAL_URL"`
	SecurityPassHash  []byte        `yaml:"-" json:"-" envconfig:"-"` // bcrypt hash, populated by HashPasswords()
	ViewerPassHash    []byte        `yaml:"-" json:"-" envconfig:"-"` // bcrypt hash, populated by HashPasswords()
}

const defaultJWTSecret = "super-secret-key-for-dev"

// LoadConfig reads configuration from three sources with the following precedence:
// env vars (highest) > YAML file > hardcoded defaults (lowest).
// The YAML file path is read from CONFIG_FILE env var (default: /app/alluredeck/config.yaml).
// A missing config file is silently ignored; a malformed file returns an error.
func LoadConfig() (*Config, error) {
	// Layer 1: Struct literal with hardcoded defaults.
	cfg := Config{
		Port:                     "8080",
		JWTSecret:                defaultJWTSecret,
		AllureVersionPath:        "/app/version",
		ProjectsPath:             "/data/projects",
		CheckResultsEverySeconds: "NONE",
		KeepHistory:              true,
		KeepHistoryLatest:        100,
		PendingResultsMaxAgeDays: 3,
		AccessTokenExpiry:        DurationSeconds(3600 * time.Second),
		RefreshTokenExpiry:       DurationSeconds(2592000 * time.Second),
		ReportGenerationTimeout:  DurationSeconds(5 * time.Minute),
		DBMaxOpenConns:           25,
		DBMaxIdleConns:           5,
		DBConnMaxLifetime:        5 * time.Minute,
		StorageType:              "local",
		LogLevel:                 "info",
		MaxUploadSizeMB:          100,
		S3: S3Config{
			Region:      "us-east-1",
			Concurrency: 10,
		},
		OIDC: OIDCConfig{
			Scopes:            []string{"openid", "profile", "email"},
			GroupsClaim:       "groups",
			DefaultRole:       "viewer",
			PostLoginRedirect: "/",
		},
	}

	// Layer 2: YAML file overrides defaults for fields present in the file.
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "/app/alluredeck/config.yaml"
	}
	configFile = filepath.Clean(configFile)
	data, err := os.ReadFile(configFile) //nolint:gosec // G703: config file path is set by the server admin via CONFIG_FILE env var, not untrusted user input
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed to read config file %q: %w", configFile, err)
		}
	} else if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", configFile, err)
	}

	// Layer 3: Environment variables override everything.
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to process environment variables: %w", err)
	}

	// Trim whitespace and filter empty entries from CORS origins.
	cfg.CORSAllowedOrigins = cleanCORSOrigins(cfg.CORSAllowedOrigins)

	return &cfg, nil
}

// ErrInsecureJWTSecret is returned when security is enabled with the default secret.
var ErrInsecureJWTSecret = errors.New("SECURITY_ENABLED is true but JWT_SECRET_KEY is the insecure default; set JWT_SECRET_KEY to a strong random secret")

// ErrS3EndpointRequired is returned when storage_type is "s3" but S3_ENDPOINT is not set.
var ErrS3EndpointRequired = errors.New("S3_ENDPOINT is required when STORAGE_TYPE=s3")

// ErrS3BucketRequired is returned when storage_type is "s3" but S3_BUCKET is not set.
var ErrS3BucketRequired = errors.New("S3_BUCKET is required when STORAGE_TYPE=s3")

// ErrOIDCIssuerRequired is returned when OIDC is enabled but IssuerURL is not set.
var ErrOIDCIssuerRequired = errors.New("OIDC_ISSUER_URL is required when OIDC_ENABLED=true")

// ErrOIDCClientIDRequired is returned when OIDC is enabled but ClientID is not set.
var ErrOIDCClientIDRequired = errors.New("OIDC_CLIENT_ID is required when OIDC_ENABLED=true")

// ErrOIDCClientSecretRequired is returned when OIDC is enabled but ClientSecret is not set.
var ErrOIDCClientSecretRequired = errors.New("OIDC_CLIENT_SECRET is required when OIDC_ENABLED=true")

// ErrOIDCRedirectURLRequired is returned when OIDC is enabled but RedirectURL is not set.
var ErrOIDCRedirectURLRequired = errors.New("OIDC_REDIRECT_URL is required when OIDC_ENABLED=true")

// ErrOIDCStateCookieSecretRequired is returned when OIDC is enabled but StateCookieSecret is not set.
var ErrOIDCStateCookieSecretRequired = errors.New("OIDC_STATE_COOKIE_SECRET is required when OIDC_ENABLED=true")

// ErrOIDCStateCookieSecretLength is returned when OIDC StateCookieSecret is not 16, 24, or 32 bytes.
var ErrOIDCStateCookieSecretLength = errors.New("OIDC_STATE_COOKIE_SECRET must be exactly 16, 24, or 32 bytes for AES-128/192/256")

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
	if c.OIDC.Enabled {
		if c.OIDC.IssuerURL == "" {
			return ErrOIDCIssuerRequired
		}
		if c.OIDC.ClientID == "" {
			return ErrOIDCClientIDRequired
		}
		if c.OIDC.ClientSecret == "" {
			return ErrOIDCClientSecretRequired
		}
		if c.OIDC.RedirectURL == "" {
			return ErrOIDCRedirectURLRequired
		}
		if c.OIDC.StateCookieSecret == "" {
			return ErrOIDCStateCookieSecretRequired
		}
		n := len(c.OIDC.StateCookieSecret)
		if n != 16 && n != 24 && n != 32 {
			return ErrOIDCStateCookieSecretLength
		}
	}
	return nil
}

// HashPasswords bcrypt-hashes AdminPass and ViewerPass into their Hash fields,
// then zeros out the plaintext fields so they cannot leak via debug endpoints or logs.
func (c *Config) HashPasswords() error {
	if c.AdminPass != "" {
		h, err := bcrypt.GenerateFromPassword([]byte(c.AdminPass), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash security password: %w", err)
		}
		c.SecurityPassHash = h
		c.AdminPass = ""
	}
	if c.ViewerPass != "" {
		h, err := bcrypt.GenerateFromPassword([]byte(c.ViewerPass), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash viewer password: %w", err)
		}
		c.ViewerPassHash = h
		c.ViewerPass = ""
	}
	return nil
}

// cleanCORSOrigins trims whitespace and removes empty entries from CORS origins.
func cleanCORSOrigins(origins []string) []string {
	var cleaned []string
	for _, o := range origins {
		o = strings.TrimSpace(o)
		if o != "" {
			cleaned = append(cleaned, o)
		}
	}
	return cleaned
}
