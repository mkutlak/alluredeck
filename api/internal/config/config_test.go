package config

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// --- Defaults ---

// TestLoadConfig_Defaults verifies the hardcoded defaults that carry a real
// security, auth, or data-retention consequence if silently regressed.
// Purely cosmetic defaults (e.g. swagger host, S3 region/concurrency) are
// intentionally not asserted here.
func TestLoadConfig_Defaults(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"KeepHistory", cfg.KeepHistory, true},
		{"CORSAllowedOrigins length", len(cfg.CORSAllowedOrigins), 0},
		{"StorageType", cfg.StorageType, "local"},
		{"KeepHistoryMaxAgeDays", cfg.KeepHistoryMaxAgeDays, 0},
		{"SwaggerEnabled", cfg.SwaggerEnabled, false},
		{"OIDC.Enabled", cfg.OIDC.Enabled, false},
		{"OIDC.DefaultRole", cfg.OIDC.DefaultRole, "viewer"},
		{"OIDC.Scopes", strings.Join(cfg.OIDC.Scopes, ","), "openid,profile,email"},
		{"OIDC.GroupsClaim", cfg.OIDC.GroupsClaim, "groups"},
		{"OIDC.PostLoginRedirect", cfg.OIDC.PostLoginRedirect, "/"},
		{"RunMigrations", cfg.RunMigrations, true},
		{"BackgroundJobsEnabled", cfg.BackgroundJobsEnabled, true},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if c.got != c.want {
				t.Errorf("expected %v, got %v", c.want, c.got)
			}
		})
	}
}

// --- Env var parsing ---

// TestLoadConfig_EnvVarParsing verifies that environment variables are read
// and mapped onto the correct Config fields, including the documented
// zero-value contracts (MIGRATION_TIMEOUT=0 disables the deadline,
// DB_IDLE_IN_TX_TIMEOUT=0s disables the GUC).
func TestLoadConfig_EnvVarParsing(t *testing.T) {
	t.Run("PortDevModeSecurityEnabled", func(t *testing.T) {
		t.Setenv("PORT", "9191")
		t.Setenv("DEV_MODE", "1")
		t.Setenv("SECURITY_ENABLED", "1")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.Port != "9191" {
			t.Errorf("Expected Port 9191, got %s", cfg.Port)
		}
		if !cfg.DevMode {
			t.Errorf("Expected DevMode true, got false")
		}
		if !cfg.SecurityEnabled {
			t.Errorf("Expected SecurityEnabled true, got false")
		}
	})

	t.Run("CORSOriginsParsed", func(t *testing.T) {
		t.Setenv("CORS_ALLOWED_ORIGINS", "https://a.example.com, https://b.example.com")
		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if len(cfg.CORSAllowedOrigins) != 2 {
			t.Errorf("Expected 2 CORS origins, got %d: %v", len(cfg.CORSAllowedOrigins), cfg.CORSAllowedOrigins)
		}
	})

	t.Run("SwaggerEnabledFromEnv", func(t *testing.T) {
		t.Setenv("SWAGGER_ENABLED", "true")
		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if !cfg.SwaggerEnabled {
			t.Errorf("Expected SwaggerEnabled true when env set, got false")
		}
	})

	t.Run("KeepHistoryMaxAgeDaysFromEnv", func(t *testing.T) {
		t.Setenv("KEEP_HISTORY_MAX_AGE_DAYS", "90")
		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.KeepHistoryMaxAgeDays != 90 {
			t.Errorf("KeepHistoryMaxAgeDays from env: want 90, got %d", cfg.KeepHistoryMaxAgeDays)
		}
	})

	t.Run("RunMigrationsFromEnv", func(t *testing.T) {
		t.Setenv("RUN_MIGRATIONS", "false")
		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.RunMigrations {
			t.Errorf("RUN_MIGRATIONS=false: want false, got true")
		}
	})

	t.Run("MigrationTimeoutZeroDisablesDeadline", func(t *testing.T) {
		t.Setenv("MIGRATION_TIMEOUT", "0")
		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.MigrationTimeout != 0 {
			t.Errorf("MIGRATION_TIMEOUT=0: want 0, got %v", cfg.MigrationTimeout)
		}
	})

	t.Run("DBTimeoutsFromEnv", func(t *testing.T) {
		t.Setenv("DB_STATEMENT_TIMEOUT", "1m")
		t.Setenv("DB_LOCK_TIMEOUT", "10s")
		t.Setenv("DB_IDLE_IN_TX_TIMEOUT", "0s")
		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.DBStatementTimeout != time.Minute {
			t.Errorf("DBStatementTimeout from env: want 1m, got %v", cfg.DBStatementTimeout)
		}
		if cfg.DBLockTimeout != 10*time.Second {
			t.Errorf("DBLockTimeout from env: want 10s, got %v", cfg.DBLockTimeout)
		}
		if cfg.DBIdleInTxTimeout != 0 {
			t.Errorf("DBIdleInTxTimeout zero: want 0, got %v", cfg.DBIdleInTxTimeout)
		}
	})

	t.Run("S3ConfigFromEnv", func(t *testing.T) {
		t.Setenv("STORAGE_TYPE", "s3")
		t.Setenv("S3_ENDPOINT", "http://minio:9000")
		t.Setenv("S3_BUCKET", "allure-reports")
		t.Setenv("S3_REGION", "eu-west-1")
		t.Setenv("S3_ACCESS_KEY", "minio-user")
		t.Setenv("S3_SECRET_KEY", "minio-password")
		t.Setenv("S3_TLS_INSECURESKIPVERIFY", "true")
		t.Setenv("S3_PATH_STYLE", "true")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}

		checks := []struct {
			name string
			got  any
			want any
		}{
			{"StorageType", cfg.StorageType, "s3"},
			{"S3.Endpoint", cfg.S3.Endpoint, "http://minio:9000"},
			{"S3.Bucket", cfg.S3.Bucket, "allure-reports"},
			{"S3.Region", cfg.S3.Region, "eu-west-1"},
			{"S3.AccessKey", cfg.S3.AccessKey, "minio-user"},
			{"S3.SecretKey", cfg.S3.SecretKey, "minio-password"},
			{"S3.TLSInsecureSkipVerify", cfg.S3.TLSInsecureSkipVerify, true},
			{"S3.PathStyle", cfg.S3.PathStyle, true},
		}
		for _, c := range checks {
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()
				if c.got != c.want {
					t.Errorf("want %v, got %v", c.want, c.got)
				}
			})
		}
	})

	t.Run("OIDCConfigFromEnv", func(t *testing.T) {
		t.Setenv("OIDC_ENABLED", "true")
		t.Setenv("OIDC_ISSUER_URL", "https://idp.example.com")
		t.Setenv("OIDC_CLIENT_ID", "my-client-id")
		t.Setenv("OIDC_CLIENT_SECRET", "my-client-secret")
		t.Setenv("OIDC_REDIRECT_URL", "https://app.example.com/callback")
		t.Setenv("OIDC_GROUPS_CLAIM", "roles")
		t.Setenv("OIDC_DEFAULT_ROLE", "viewer")
		t.Setenv("OIDC_STATE_COOKIE_SECRET", "12345678901234567890123456789012")
		t.Setenv("OIDC_POST_LOGIN_REDIRECT", "/dashboard")
		t.Setenv("OIDC_END_SESSION_URL", "https://idp.example.com/logout")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}

		checks := []struct {
			name string
			got  any
			want any
		}{
			{"OIDC.Enabled", cfg.OIDC.Enabled, true},
			{"OIDC.IssuerURL", cfg.OIDC.IssuerURL, "https://idp.example.com"},
			{"OIDC.ClientID", cfg.OIDC.ClientID, "my-client-id"},
			{"OIDC.ClientSecret", cfg.OIDC.ClientSecret, "my-client-secret"},
			{"OIDC.RedirectURL", cfg.OIDC.RedirectURL, "https://app.example.com/callback"},
			{"OIDC.GroupsClaim", cfg.OIDC.GroupsClaim, "roles"},
			{"OIDC.DefaultRole", cfg.OIDC.DefaultRole, "viewer"},
			{"OIDC.StateCookieSecret", cfg.OIDC.StateCookieSecret, "12345678901234567890123456789012"},
			{"OIDC.PostLoginRedirect", cfg.OIDC.PostLoginRedirect, "/dashboard"},
			{"OIDC.EndSessionURL", cfg.OIDC.EndSessionURL, "https://idp.example.com/logout"},
		}
		for _, c := range checks {
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()
				if c.got != c.want {
					t.Errorf("want %v, got %v", c.want, c.got)
				}
			})
		}
	})
}

// --- YAML loading and precedence ---

// TestLoadConfig_YAML covers CONFIG_FILE-driven loading: a fully populated
// file, env-over-YAML precedence, partial files falling back to defaults,
// missing files (named and default path) being silently ignored, and a
// malformed file returning an error.
func TestLoadConfig_YAML(t *testing.T) {
	t.Run("FullFile", func(t *testing.T) {
		t.Setenv("CONFIG_FILE", "testdata/full.yaml")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}

		checks := []struct {
			name string
			got  any
			want any
		}{
			{"Port", cfg.Port, "9090"},
			{"DevMode", cfg.DevMode, true},
			{"SecurityEnabled", cfg.SecurityEnabled, true},
			{"AdminUser", cfg.AdminUser, "yaml-admin"},
			{"AdminPass", cfg.AdminPass, "yaml-pass"},
			{"ViewerUser", cfg.ViewerUser, "yaml-viewer"},
			{"ViewerPass", cfg.ViewerPass, "yaml-viewpass"},
			{"JWTSecret", cfg.JWTSecret, "yaml-jwt-secret-that-is-long-enough"},
			{"MakeViewerEndpointsPublic", cfg.MakeViewerEndpointsPublic, true},
			{"AllureVersionPath", cfg.AllureVersionPath, "/yaml-version"},
			{"ProjectsPath", cfg.ProjectsPath, "/yaml/projects"},
			{"CheckResultsEverySeconds", cfg.CheckResultsEverySeconds, "5"},
			{"KeepHistory", cfg.KeepHistory, true},
			{"KeepHistoryLatest", cfg.KeepHistoryLatest, 50},
			{"KeepHistoryMaxAgeDays", cfg.KeepHistoryMaxAgeDays, 30},
			{"TLS", cfg.TLS, true},
			{"APIResponseLessVerbose", cfg.APIResponseLessVerbose, true},
			{"AccessTokenExpiry", cfg.AccessTokenExpiry, DurationSeconds(1800 * time.Second)},
			{"RefreshTokenExpiry", cfg.RefreshTokenExpiry, DurationSeconds(86400 * time.Second)},
			{"CORSAllowedOrigins length", len(cfg.CORSAllowedOrigins), 2},
			{"StorageType", cfg.StorageType, "s3"},
			{"S3.Endpoint", cfg.S3.Endpoint, "http://minio-test:9000"},
			{"S3.Bucket", cfg.S3.Bucket, "test-bucket"},
			{"S3.Region", cfg.S3.Region, "eu-central-1"},
			{"S3.AccessKey", cfg.S3.AccessKey, "test-access-key"},
			{"S3.SecretKey", cfg.S3.SecretKey, "test-secret-key"},
			{"S3.TLSInsecureSkipVerify", cfg.S3.TLSInsecureSkipVerify, true},
			{"S3.PathStyle", cfg.S3.PathStyle, true},
			{"S3.Concurrency", cfg.S3.Concurrency, 5},
		}
		for _, c := range checks {
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()
				if c.got != c.want {
					t.Errorf("expected %v, got %v", c.want, c.got)
				}
			})
		}

		// Exact CORS values (length already checked above).
		if cfg.CORSAllowedOrigins[0] != "https://a.example.com" {
			t.Errorf("CORS[0]: want https://a.example.com, got %s", cfg.CORSAllowedOrigins[0])
		}
		if cfg.CORSAllowedOrigins[1] != "https://b.example.com" {
			t.Errorf("CORS[1]: want https://b.example.com, got %s", cfg.CORSAllowedOrigins[1])
		}
	})

	t.Run("EnvOverridesYAML", func(t *testing.T) {
		t.Setenv("CONFIG_FILE", "testdata/full.yaml")
		t.Setenv("PORT", "1111")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}

		if cfg.Port != "1111" {
			t.Errorf("env PORT should override YAML: want 1111, got %s", cfg.Port)
		}
		// Other YAML values remain
		if cfg.KeepHistoryLatest != 50 {
			t.Errorf("YAML KeepHistoryLatest should be 50, got %d", cfg.KeepHistoryLatest)
		}
	})

	t.Run("PartialYAMLWithDefaults", func(t *testing.T) {
		t.Setenv("CONFIG_FILE", "testdata/partial.yaml")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}

		if cfg.Port != "7777" {
			t.Errorf("partial YAML port: want 7777, got %s", cfg.Port)
		}
		if !cfg.KeepHistory {
			t.Errorf("partial YAML keep_history: want true, got false")
		}
		// Unset fields get hardcoded defaults
		if cfg.KeepHistoryLatest != 100 {
			t.Errorf("default KeepHistoryLatest: want 100, got %d", cfg.KeepHistoryLatest)
		}
	})

	t.Run("MissingNamedFileNotError", func(t *testing.T) {
		t.Setenv("CONFIG_FILE", "testdata/does-not-exist.yaml")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}

		// Must not panic/fatal; defaults apply
		if cfg.Port != "8080" {
			t.Errorf("default port: want 8080, got %s", cfg.Port)
		}
	})

	t.Run("DefaultPathMissing", func(t *testing.T) {
		// No CONFIG_FILE set; default /app/alluredeck/config.yaml doesn't exist in test env
		orig, wasSet := os.LookupEnv("CONFIG_FILE")
		_ = os.Unsetenv("CONFIG_FILE")
		if wasSet {
			t.Cleanup(func() { _ = os.Setenv("CONFIG_FILE", orig) })
		} else {
			t.Cleanup(func() { _ = os.Unsetenv("CONFIG_FILE") })
		}

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}

		if cfg.Port != "8080" {
			t.Errorf("default port: want 8080, got %s", cfg.Port)
		}
	})

	t.Run("MalformedFileErrors", func(t *testing.T) {
		t.Setenv("CONFIG_FILE", "testdata/malformed.yaml")
		_, err := LoadConfig()
		if err == nil {
			t.Error("expected error for malformed YAML, got nil")
		}
	})

	t.Run("EmptyFileUsesDefaults", func(t *testing.T) {
		t.Setenv("CONFIG_FILE", "testdata/empty.yaml")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}

		if cfg.Port != "8080" {
			t.Errorf("empty YAML should use default port 8080, got %s", cfg.Port)
		}
		if cfg.KeepHistoryLatest != 100 {
			t.Errorf("empty YAML should use default KeepHistoryLatest 100, got %d", cfg.KeepHistoryLatest)
		}
	})
}

// --- Validate() rules ---

// TestValidate covers every rejection and acceptance path in Config.Validate:
// DatabaseURL shape, the insecure default JWT secret, S3 requirements, MCP
// server requirements, and the full OIDC field-by-field validation chain.
func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr error // nil means Validate() must return nil
	}{
		{
			name:    "DatabaseURLEmpty",
			cfg:     &Config{DatabaseURL: "", JWTSecret: "some-safe-secret"},
			wantErr: ErrDatabaseURLRequired,
		},
		{
			name:    "DatabaseURLInvalid",
			cfg:     &Config{DatabaseURL: "not-a-dsn", JWTSecret: "some-safe-secret"},
			wantErr: ErrDatabaseURLInvalid,
		},
		{
			name: "DatabaseURLPostgresScheme",
			cfg:  &Config{DatabaseURL: "postgres://localhost/mydb", JWTSecret: "some-safe-secret"},
		},
		{
			name: "DatabaseURLPostgresqlScheme",
			cfg:  &Config{DatabaseURL: "postgresql://user:pass@host/db", JWTSecret: "some-safe-secret"},
		},
		{
			name: "DatabaseURLKeywordForm",
			cfg:  &Config{DatabaseURL: "host=localhost dbname=mydb user=app", JWTSecret: "some-safe-secret"},
		},
		{
			// Anyone who reads the source could forge tokens signed with the
			// shipped default, so security must refuse to start with it.
			name:    "InsecureDefaultJWTSecretWithSecurity",
			cfg:     &Config{DatabaseURL: "postgres://localhost/test", SecurityEnabled: true, JWTSecret: defaultJWTSecret},
			wantErr: ErrInsecureJWTSecret,
		},
		{
			// The guard applies only when security is enabled.
			name: "DefaultJWTSecretWithoutSecurity",
			cfg:  &Config{DatabaseURL: "postgres://localhost/test", SecurityEnabled: false, JWTSecret: defaultJWTSecret},
		},
		{
			name: "StrongJWTSecretWithSecurity",
			cfg:  &Config{DatabaseURL: "postgres://localhost/test", SecurityEnabled: true, JWTSecret: "some-safe-secret"},
		},
		{
			name: "S3RequiresEndpoint",
			cfg: &Config{
				DatabaseURL: "postgres://localhost/test",
				StorageType: "s3",
				S3:          S3Config{Bucket: "my-bucket"},
				JWTSecret:   "some-safe-secret",
			},
			wantErr: ErrS3EndpointRequired,
		},
		{
			name: "S3RequiresBucket",
			cfg: &Config{
				DatabaseURL: "postgres://localhost/test",
				StorageType: "s3",
				S3:          S3Config{Endpoint: "http://minio:9000"},
				JWTSecret:   "some-safe-secret",
			},
			wantErr: ErrS3BucketRequired,
		},
		{
			name: "S3WithFullConfig",
			cfg: &Config{
				DatabaseURL: "postgres://localhost/test",
				StorageType: "s3",
				S3:          S3Config{Endpoint: "http://minio:9000", Bucket: "allure-reports"},
				JWTSecret:   "some-safe-secret",
			},
		},
		{
			name: "LocalStorageNoS3Required",
			cfg: &Config{
				DatabaseURL: "postgres://localhost/test",
				StorageType: "local",
				JWTSecret:   "some-safe-secret",
			},
		},
		{
			name: "MCPServerRequiresExternalURL",
			cfg: &Config{
				DatabaseURL:      "postgres://localhost/test",
				JWTSecret:        "some-safe-secret",
				MCPServerEnabled: true,
				ExternalURL:      "",
			},
			wantErr: ErrExternalURLRequired,
		},
		{
			name: "MCPServerWithExternalURLPasses",
			cfg: &Config{
				DatabaseURL:      "postgres://localhost/test",
				JWTSecret:        "some-safe-secret",
				MCPServerEnabled: true,
				ExternalURL:      "https://alluredeck.example.com",
			},
		},
		{
			name: "OIDCDisabled",
			cfg: &Config{
				DatabaseURL: "postgres://localhost/test",
				JWTSecret:   "some-safe-secret",
				OIDC:        OIDCConfig{Enabled: false},
			},
		},
		{
			name: "OIDCMissingIssuer",
			cfg: &Config{
				DatabaseURL: "postgres://localhost/test",
				JWTSecret:   "some-safe-secret",
				OIDC:        OIDCConfig{Enabled: true},
			},
			wantErr: ErrOIDCIssuerRequired,
		},
		{
			name: "OIDCMissingClientID",
			cfg: &Config{
				DatabaseURL: "postgres://localhost/test",
				JWTSecret:   "some-safe-secret",
				OIDC: OIDCConfig{
					Enabled:   true,
					IssuerURL: "https://idp.example.com",
				},
			},
			wantErr: ErrOIDCClientIDRequired,
		},
		{
			name: "OIDCMissingClientSecret",
			cfg: &Config{
				DatabaseURL: "postgres://localhost/test",
				JWTSecret:   "some-safe-secret",
				OIDC: OIDCConfig{
					Enabled:   true,
					IssuerURL: "https://idp.example.com",
					ClientID:  "my-client-id",
				},
			},
			wantErr: ErrOIDCClientSecretRequired,
		},
		{
			name: "OIDCMissingRedirectURL",
			cfg: &Config{
				DatabaseURL: "postgres://localhost/test",
				JWTSecret:   "some-safe-secret",
				OIDC: OIDCConfig{
					Enabled:      true,
					IssuerURL:    "https://idp.example.com",
					ClientID:     "my-client-id",
					ClientSecret: "my-client-secret",
				},
			},
			wantErr: ErrOIDCRedirectURLRequired,
		},
		{
			name: "OIDCMissingStateCookieSecret",
			cfg: &Config{
				DatabaseURL: "postgres://localhost/test",
				JWTSecret:   "some-safe-secret",
				OIDC: OIDCConfig{
					Enabled:      true,
					IssuerURL:    "https://idp.example.com",
					ClientID:     "my-client-id",
					ClientSecret: "my-client-secret",
					RedirectURL:  "https://app.example.com/callback",
				},
			},
			wantErr: ErrOIDCStateCookieSecretRequired,
		},
		{
			name: "OIDCBadSecretLength",
			cfg: &Config{
				DatabaseURL: "postgres://localhost/test",
				JWTSecret:   "some-safe-secret",
				OIDC: OIDCConfig{
					Enabled:           true,
					IssuerURL:         "https://idp.example.com",
					ClientID:          "my-client-id",
					ClientSecret:      "my-client-secret",
					RedirectURL:       "https://app.example.com/callback",
					StateCookieSecret: "tooshort",
				},
			},
			wantErr: ErrOIDCStateCookieSecretLength,
		},
		{
			name: "OIDCValidConfig",
			cfg: &Config{
				DatabaseURL: "postgres://localhost/test",
				JWTSecret:   "some-safe-secret",
				OIDC: OIDCConfig{
					Enabled:           true,
					IssuerURL:         "https://idp.example.com",
					ClientID:          "my-client-id",
					ClientSecret:      "my-client-secret",
					RedirectURL:       "https://app.example.com/callback",
					StateCookieSecret: "12345678901234567890123456789012", // 32 bytes
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

// --- HashPasswords (security: Always-keep, merged into one table) ---

func TestHashPasswords(t *testing.T) {
	t.Run("ClearsPlaintext", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			AdminPass:  "admin-secret",
			ViewerPass: "viewer-secret",
		}
		if err := cfg.HashPasswords(); err != nil {
			t.Fatalf("HashPasswords: %v", err)
		}
		if cfg.AdminPass != "" {
			t.Errorf("AdminPass should be empty after hashing, got %q", cfg.AdminPass)
		}
		if cfg.ViewerPass != "" {
			t.Errorf("ViewerPass should be empty after hashing, got %q", cfg.ViewerPass)
		}
		if len(cfg.SecurityPassHash) == 0 {
			t.Error("SecurityPassHash should be populated")
		}
		if len(cfg.ViewerPassHash) == 0 {
			t.Error("ViewerPassHash should be populated")
		}
	})

	t.Run("CorrectPasswordVerifies", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			AdminPass:  "admin-secret",
			ViewerPass: "viewer-secret",
		}
		if err := cfg.HashPasswords(); err != nil {
			t.Fatalf("HashPasswords: %v", err)
		}
		if err := bcrypt.CompareHashAndPassword(cfg.SecurityPassHash, []byte("admin-secret")); err != nil {
			t.Errorf("admin password should match: %v", err)
		}
		if err := bcrypt.CompareHashAndPassword(cfg.ViewerPassHash, []byte("viewer-secret")); err != nil {
			t.Errorf("viewer password should match: %v", err)
		}
	})

	t.Run("WrongPasswordRejected", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			AdminPass: "admin-secret",
		}
		if err := cfg.HashPasswords(); err != nil {
			t.Fatalf("HashPasswords: %v", err)
		}
		if err := bcrypt.CompareHashAndPassword(cfg.SecurityPassHash, []byte("wrong-password")); err == nil {
			t.Error("wrong password should not match")
		}
	})

	t.Run("EmptyPasswordsNoOp", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{}
		if err := cfg.HashPasswords(); err != nil {
			t.Fatalf("HashPasswords: %v", err)
		}
		if len(cfg.SecurityPassHash) != 0 {
			t.Error("SecurityPassHash should be empty when no password set")
		}
		if len(cfg.ViewerPassHash) != 0 {
			t.Error("ViewerPassHash should be empty when no password set")
		}
	})
}

// --- DurationSeconds.Decode ---

func TestDurationSeconds_Decode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{name: "IntegerSeconds", input: "900", want: 900 * time.Second},
		{name: "GoDuration", input: "15m", want: 15 * time.Minute},
		{name: "InvalidValue", input: "invalid", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var d DurationSeconds
			err := d.Decode(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Decode(%q): %v", tt.input, err)
			}
			if d.Duration() != tt.want {
				t.Errorf("expected %v, got %v", tt.want, d.Duration())
			}
			if d.Seconds() != tt.want.Seconds() {
				t.Errorf("expected %v seconds, got %v", tt.want.Seconds(), d.Seconds())
			}
		})
	}
}
