package config

import (
	"errors"
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestLoadConfig(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("DEV_MODE", "1")
	t.Setenv("SECURITY_ENABLED", "1")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Port != "8080" {
		t.Errorf("Expected Port 8080, got %s", cfg.Port)
	}

	if !cfg.DevMode {
		t.Errorf("Expected DevMode true, got false")
	}

	if !cfg.SecurityEnabled {
		t.Errorf("Expected SecurityEnabled true, got false")
	}
}

func TestKeepHistoryDefaultTrue(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.KeepHistory {
		t.Errorf("Expected KeepHistory default true, got false")
	}
}

func TestCORSOriginsEmpty(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.CORSAllowedOrigins) != 0 {
		t.Errorf("Expected no CORS origins by default, got %v", cfg.CORSAllowedOrigins)
	}
}

func TestCORSOriginsParsed(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://a.example.com, https://b.example.com")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Errorf("Expected 2 CORS origins, got %d: %v", len(cfg.CORSAllowedOrigins), cfg.CORSAllowedOrigins)
	}
}

// --- YAML config tests ---

func TestLoadConfigFromYAMLFile(t *testing.T) {
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

func TestEnvVarOverridesYAML(t *testing.T) {
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
}

func TestPartialYAMLWithDefaults(t *testing.T) {
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
}

func TestMissingConfigFileNotError(t *testing.T) {
	t.Setenv("CONFIG_FILE", "testdata/does-not-exist.yaml")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// Must not panic/fatal; defaults apply
	if cfg.Port != "8080" {
		t.Errorf("default port: want 8080, got %s", cfg.Port)
	}
}

func TestDefaultConfigPathMissing(t *testing.T) {
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
}

func TestMalformedYAML(t *testing.T) {
	t.Setenv("CONFIG_FILE", "testdata/malformed.yaml")
	_, err := LoadConfig()
	if err == nil {
		t.Error("expected error for malformed YAML, got nil")
	}
}

func TestEmptyYAMLFile(t *testing.T) {
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
}

func TestYAMLCORSOrigins(t *testing.T) {
	t.Setenv("CONFIG_FILE", "testdata/full.yaml")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("expected 2 CORS origins from YAML, got %d: %v", len(cfg.CORSAllowedOrigins), cfg.CORSAllowedOrigins)
	}
	if cfg.CORSAllowedOrigins[0] != "https://a.example.com" {
		t.Errorf("CORS[0]: want https://a.example.com, got %s", cfg.CORSAllowedOrigins[0])
	}
	if cfg.CORSAllowedOrigins[1] != "https://b.example.com" {
		t.Errorf("CORS[1]: want https://b.example.com, got %s", cfg.CORSAllowedOrigins[1])
	}
}

func TestYAMLTokenExpiry(t *testing.T) {
	t.Setenv("CONFIG_FILE", "testdata/full.yaml")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.AccessTokenExpiry != DurationSeconds(1800*time.Second) {
		t.Errorf("AccessTokenExpiry: want 1800s, got %v", cfg.AccessTokenExpiry)
	}
	if cfg.RefreshTokenExpiry != DurationSeconds(86400*time.Second) {
		t.Errorf("RefreshTokenExpiry: want 86400s, got %v", cfg.RefreshTokenExpiry)
	}
}

func TestStorageTypeDefault(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.StorageType != "local" {
		t.Errorf("default StorageType: want %q, got %q", "local", cfg.StorageType)
	}
}

func TestStorageTypeFromEnv(t *testing.T) {
	t.Setenv("STORAGE_TYPE", "s3")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.StorageType != "s3" {
		t.Errorf("StorageType from env: want %q, got %q", "s3", cfg.StorageType)
	}
}

func TestS3ConfigFromEnv(t *testing.T) {
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
}

func TestValidateS3RequiresEndpoint(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		StorageType: "s3",
		S3:          S3Config{Bucket: "my-bucket"},
		JWTSecret:   "some-safe-secret",
	}
	err := cfg.Validate()
	if !errors.Is(err, ErrS3EndpointRequired) {
		t.Errorf("expected ErrS3EndpointRequired, got %v", err)
	}
}

func TestValidateS3RequiresBucket(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		StorageType: "s3",
		S3:          S3Config{Endpoint: "http://minio:9000"},
		JWTSecret:   "some-safe-secret",
	}
	err := cfg.Validate()
	if !errors.Is(err, ErrS3BucketRequired) {
		t.Errorf("expected ErrS3BucketRequired, got %v", err)
	}
}

func TestValidateS3WithFullConfig(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		StorageType: "s3",
		S3: S3Config{
			Endpoint: "http://minio:9000",
			Bucket:   "allure-reports",
		},
		JWTSecret: "some-safe-secret",
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for valid S3 config, got %v", err)
	}
}

func TestValidateLocalStorageNoS3Required(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		StorageType: "local",
		JWTSecret:   "some-safe-secret",
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for local storage, got %v", err)
	}
}

func TestS3RegionDefault(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.S3.Region != "us-east-1" {
		t.Errorf("default S3 region: want %q, got %q", "us-east-1", cfg.S3.Region)
	}
}

func TestLoadConfigS3FromYAMLFile(t *testing.T) {
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
				t.Errorf("want %v, got %v", c.want, c.got)
			}
		})
	}
}

func TestS3ConcurrencyDefault(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.S3.Concurrency != 10 {
		t.Errorf("default S3 concurrency: want 10, got %d", cfg.S3.Concurrency)
	}
}

func TestS3ConcurrencyFromEnv(t *testing.T) {
	t.Setenv("S3_CONCURRENCY", "20")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.S3.Concurrency != 20 {
		t.Errorf("S3 concurrency from env: want 20, got %d", cfg.S3.Concurrency)
	}
}

func TestKeepHistoryMaxAgeDaysDefault(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.KeepHistoryMaxAgeDays != 0 {
		t.Errorf("default KeepHistoryMaxAgeDays: want 0, got %d", cfg.KeepHistoryMaxAgeDays)
	}
}

func TestKeepHistoryMaxAgeDaysFromEnv(t *testing.T) {
	t.Setenv("KEEP_HISTORY_MAX_AGE_DAYS", "90")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.KeepHistoryMaxAgeDays != 90 {
		t.Errorf("KeepHistoryMaxAgeDays from env: want 90, got %d", cfg.KeepHistoryMaxAgeDays)
	}
}

func TestHashPasswords_ClearsPlaintext(t *testing.T) {
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
}

func TestHashPasswords_CorrectPasswordVerifies(t *testing.T) {
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
}

func TestHashPasswords_WrongPasswordRejected(t *testing.T) {
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
}

func TestHashPasswords_EmptyPasswordsNoOp(t *testing.T) {
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
}

func TestSwaggerEnabledDefaultFalse(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.SwaggerEnabled {
		t.Errorf("Expected SwaggerEnabled default false, got true")
	}
}

func TestSwaggerEnabledFromEnv(t *testing.T) {
	t.Setenv("SWAGGER_ENABLED", "true")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.SwaggerEnabled {
		t.Errorf("Expected SwaggerEnabled true when env set, got false")
	}
}

func TestSwaggerHostDefaultEmpty(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.SwaggerHost != "" {
		t.Errorf("Expected SwaggerHost default empty, got %q", cfg.SwaggerHost)
	}
}

func TestSwaggerHostFromEnv(t *testing.T) {
	t.Setenv("SWAGGER_HOST", "alluredeck.example.com")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.SwaggerHost != "alluredeck.example.com" {
		t.Errorf("Expected SwaggerHost %q, got %q", "alluredeck.example.com", cfg.SwaggerHost)
	}
}

func TestOIDCConfig_Defaults(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.OIDC.Enabled {
		t.Errorf("default OIDC.Enabled: want false, got true")
	}
	if len(cfg.OIDC.Scopes) != 3 || cfg.OIDC.Scopes[0] != "openid" || cfg.OIDC.Scopes[1] != "profile" || cfg.OIDC.Scopes[2] != "email" {
		t.Errorf("default OIDC.Scopes: want [openid profile email], got %v", cfg.OIDC.Scopes)
	}
	if cfg.OIDC.GroupsClaim != "groups" {
		t.Errorf("default OIDC.GroupsClaim: want %q, got %q", "groups", cfg.OIDC.GroupsClaim)
	}
	if cfg.OIDC.DefaultRole != "viewer" {
		t.Errorf("default OIDC.DefaultRole: want %q, got %q", "viewer", cfg.OIDC.DefaultRole)
	}
	if cfg.OIDC.PostLoginRedirect != "/" {
		t.Errorf("default OIDC.PostLoginRedirect: want %q, got %q", "/", cfg.OIDC.PostLoginRedirect)
	}
}

func TestOIDCConfig_EnvVars(t *testing.T) {
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
}

func TestOIDCConfig_Validate_Disabled(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		JWTSecret: "some-safe-secret",
		OIDC:      OIDCConfig{Enabled: false},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error when OIDC disabled, got %v", err)
	}
}

func TestOIDCConfig_Validate_MissingIssuer(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		JWTSecret: "some-safe-secret",
		OIDC: OIDCConfig{
			Enabled: true,
		},
	}
	if err := cfg.Validate(); !errors.Is(err, ErrOIDCIssuerRequired) {
		t.Errorf("expected ErrOIDCIssuerRequired, got %v", err)
	}
}

func TestOIDCConfig_Validate_MissingClientID(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		JWTSecret: "some-safe-secret",
		OIDC: OIDCConfig{
			Enabled:   true,
			IssuerURL: "https://idp.example.com",
		},
	}
	if err := cfg.Validate(); !errors.Is(err, ErrOIDCClientIDRequired) {
		t.Errorf("expected ErrOIDCClientIDRequired, got %v", err)
	}
}

func TestOIDCConfig_Validate_MissingClientSecret(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		JWTSecret: "some-safe-secret",
		OIDC: OIDCConfig{
			Enabled:   true,
			IssuerURL: "https://idp.example.com",
			ClientID:  "my-client-id",
		},
	}
	if err := cfg.Validate(); !errors.Is(err, ErrOIDCClientSecretRequired) {
		t.Errorf("expected ErrOIDCClientSecretRequired, got %v", err)
	}
}

func TestOIDCConfig_Validate_MissingRedirectURL(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		JWTSecret: "some-safe-secret",
		OIDC: OIDCConfig{
			Enabled:      true,
			IssuerURL:    "https://idp.example.com",
			ClientID:     "my-client-id",
			ClientSecret: "my-client-secret",
		},
	}
	if err := cfg.Validate(); !errors.Is(err, ErrOIDCRedirectURLRequired) {
		t.Errorf("expected ErrOIDCRedirectURLRequired, got %v", err)
	}
}

func TestOIDCConfig_Validate_MissingStateCookieSecret(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		JWTSecret: "some-safe-secret",
		OIDC: OIDCConfig{
			Enabled:      true,
			IssuerURL:    "https://idp.example.com",
			ClientID:     "my-client-id",
			ClientSecret: "my-client-secret",
			RedirectURL:  "https://app.example.com/callback",
		},
	}
	if err := cfg.Validate(); !errors.Is(err, ErrOIDCStateCookieSecretRequired) {
		t.Errorf("expected ErrOIDCStateCookieSecretRequired, got %v", err)
	}
}

func TestOIDCConfig_Validate_BadSecretLength(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		JWTSecret: "some-safe-secret",
		OIDC: OIDCConfig{
			Enabled:           true,
			IssuerURL:         "https://idp.example.com",
			ClientID:          "my-client-id",
			ClientSecret:      "my-client-secret",
			RedirectURL:       "https://app.example.com/callback",
			StateCookieSecret: "tooshort",
		},
	}
	if err := cfg.Validate(); !errors.Is(err, ErrOIDCStateCookieSecretLength) {
		t.Errorf("expected ErrOIDCStateCookieSecretLength, got %v", err)
	}
}

func TestOIDCConfig_Validate_ValidConfig(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		JWTSecret: "some-safe-secret",
		OIDC: OIDCConfig{
			Enabled:           true,
			IssuerURL:         "https://idp.example.com",
			ClientID:          "my-client-id",
			ClientSecret:      "my-client-secret",
			RedirectURL:       "https://app.example.com/callback",
			StateCookieSecret: "12345678901234567890123456789012", // 32 bytes
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for valid OIDC config, got %v", err)
	}
}

func TestDurationSeconds_IntegerSeconds(t *testing.T) {
	t.Parallel()
	var d DurationSeconds
	if err := d.Decode("900"); err != nil {
		t.Fatalf("Decode(\"900\"): %v", err)
	}
	if d.Duration() != 900*time.Second {
		t.Errorf("expected 900s, got %v", d.Duration())
	}
	if d.Seconds() != 900 {
		t.Errorf("expected 900, got %v", d.Seconds())
	}
}

func TestDurationSeconds_GoDuration(t *testing.T) {
	t.Parallel()
	var d DurationSeconds
	if err := d.Decode("15m"); err != nil {
		t.Fatalf("Decode(\"15m\"): %v", err)
	}
	if d.Duration() != 15*time.Minute {
		t.Errorf("expected 15m, got %v", d.Duration())
	}
}

func TestDurationSeconds_InvalidValue(t *testing.T) {
	t.Parallel()
	var d DurationSeconds
	if err := d.Decode("invalid"); err == nil {
		t.Error("expected error for invalid duration, got nil")
	}
}
