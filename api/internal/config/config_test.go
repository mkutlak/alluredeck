package config

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("DEV_MODE", "1")
	t.Setenv("SECURITY_ENABLED", "1")

	cfg := LoadConfig()

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

func TestGetEnvAsInt(t *testing.T) {
	t.Setenv("TEST_INT", "42")

	val := getEnvAsInt("TEST_INT", 0)
	if val != 42 {
		t.Errorf("Expected 42, got %d", val)
	}

	valDefault := getEnvAsInt("NON_EXISTENT", 99)
	if valDefault != 99 {
		t.Errorf("Expected default 99, got %d", valDefault)
	}
}

func TestGetEnvAsBool(t *testing.T) {
	cases := []struct {
		value    string
		expected bool
	}{
		{"1", true},
		{"TRUE", true},
		{"true", true},
		{"YES", true},
		{"0", false},
		{"FALSE", false},
		{"false", false},
		{"NO", false},
	}

	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("TEST_BOOL", tc.value)
			got := getEnvAsBool("TEST_BOOL", !tc.expected)
			if got != tc.expected {
				t.Errorf("getEnvAsBool with %q: expected %v, got %v", tc.value, tc.expected, got)
			}
		})
	}
}

func TestCORSOriginsEmpty(t *testing.T) {
	cfg := LoadConfig()
	if len(cfg.CORSAllowedOrigins) != 0 {
		t.Errorf("Expected no CORS origins by default, got %v", cfg.CORSAllowedOrigins)
	}
}

func TestCORSOriginsParsed(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://a.example.com, https://b.example.com")
	cfg := LoadConfig()
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Errorf("Expected 2 CORS origins, got %d: %v", len(cfg.CORSAllowedOrigins), cfg.CORSAllowedOrigins)
	}
}

// --- YAML config tests ---

func TestLoadConfigFromYAMLFile(t *testing.T) {
	t.Setenv("CONFIG_FILE", "testdata/full.yaml")

	cfg := LoadConfig()

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"Port", cfg.Port, "9090"},
		{"DevMode", cfg.DevMode, true},
		{"SecurityEnabled", cfg.SecurityEnabled, true},
		{"SecurityUser", cfg.SecurityUser, "yaml-admin"},
		{"SecurityPass", cfg.SecurityPass, "yaml-pass"},
		{"ViewerUser", cfg.ViewerUser, "yaml-viewer"},
		{"ViewerPass", cfg.ViewerPass, "yaml-viewpass"},
		{"JWTSecret", cfg.JWTSecret, "yaml-jwt-secret-that-is-long-enough"},
		{"MakeViewerEndptsPub", cfg.MakeViewerEndptsPub, true},
		{"AllureVersionPath", cfg.AllureVersionPath, "/yaml-version"},
		{"StaticContentPath", cfg.StaticContentPath, "/yaml/static"},
		{"ProjectsDirectory", cfg.ProjectsDirectory, "/yaml/projects"},
		{"CheckResultsSecs", cfg.CheckResultsSecs, "5"},
		{"KeepHistory", cfg.KeepHistory, true},
		{"KeepHistoryLatest", cfg.KeepHistoryLatest, 50},
		{"TLS", cfg.TLS, true},
		{"URLPrefix", cfg.URLPrefix, "/allure"},
		{"APIRespLessVerbose", cfg.APIRespLessVerbose, true},
		{"OptimizeStorage", cfg.OptimizeStorage, true},
		{"LegacyUI", cfg.LegacyUI, true},
		{"DatabasePath", cfg.DatabasePath, "/yaml/allure.db"},
		{"AccessTokenExpiry", cfg.AccessTokenExpiry, 1800 * time.Second},
		{"RefreshTokenExpiry", cfg.RefreshTokenExpiry, 86400 * time.Second},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("expected %v, got %v", c.want, c.got)
			}
		})
	}
}

func TestEnvVarOverridesYAML(t *testing.T) {
	t.Setenv("CONFIG_FILE", "testdata/full.yaml")
	t.Setenv("PORT", "1111")

	cfg := LoadConfig()

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

	cfg := LoadConfig()

	if cfg.Port != "7777" {
		t.Errorf("partial YAML port: want 7777, got %s", cfg.Port)
	}
	if !cfg.KeepHistory {
		t.Errorf("partial YAML keep_history: want true, got false")
	}
	// Unset fields get hardcoded defaults
	if cfg.KeepHistoryLatest != 20 {
		t.Errorf("default KeepHistoryLatest: want 20, got %d", cfg.KeepHistoryLatest)
	}
	if cfg.DatabasePath != "/app/allure.db" {
		t.Errorf("default DatabasePath: want /app/allure.db, got %s", cfg.DatabasePath)
	}
}

func TestMissingConfigFileNotError(t *testing.T) {
	t.Setenv("CONFIG_FILE", "testdata/does-not-exist.yaml")

	cfg := LoadConfig()

	// Must not panic/fatal; defaults apply
	if cfg.Port != "5050" {
		t.Errorf("default port: want 5050, got %s", cfg.Port)
	}
}

func TestDefaultConfigPathMissing(t *testing.T) {
	// No CONFIG_FILE set; default /app/config.yaml doesn't exist in test env
	orig, wasSet := os.LookupEnv("CONFIG_FILE")
	os.Unsetenv("CONFIG_FILE")
	if wasSet {
		t.Cleanup(func() { _ = os.Setenv("CONFIG_FILE", orig) })
	} else {
		t.Cleanup(func() { os.Unsetenv("CONFIG_FILE") })
	}

	cfg := LoadConfig()

	if cfg.Port != "5050" {
		t.Errorf("default port: want 5050, got %s", cfg.Port)
	}
}

func TestMalformedYAML(t *testing.T) {
	_, err := loadFromYAML("testdata/malformed.yaml")
	if err == nil {
		t.Error("expected error for malformed YAML, got nil")
	}
}

func TestEmptyYAMLFile(t *testing.T) {
	t.Setenv("CONFIG_FILE", "testdata/empty.yaml")

	cfg := LoadConfig()

	if cfg.Port != "5050" {
		t.Errorf("empty YAML should use default port 5050, got %s", cfg.Port)
	}
	if cfg.KeepHistoryLatest != 20 {
		t.Errorf("empty YAML should use default KeepHistoryLatest 20, got %d", cfg.KeepHistoryLatest)
	}
}

func TestYAMLCORSOrigins(t *testing.T) {
	t.Setenv("CONFIG_FILE", "testdata/full.yaml")

	cfg := LoadConfig()

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

	cfg := LoadConfig()

	if cfg.AccessTokenExpiry != 1800*time.Second {
		t.Errorf("AccessTokenExpiry: want 1800s, got %v", cfg.AccessTokenExpiry)
	}
	if cfg.RefreshTokenExpiry != 86400*time.Second {
		t.Errorf("RefreshTokenExpiry: want 86400s, got %v", cfg.RefreshTokenExpiry)
	}
}

func TestStorageTypeDefault(t *testing.T) {
	cfg := LoadConfig()
	if cfg.StorageType != "local" {
		t.Errorf("default StorageType: want %q, got %q", "local", cfg.StorageType)
	}
}

func TestStorageTypeFromEnv(t *testing.T) {
	t.Setenv("STORAGE_TYPE", "s3")
	cfg := LoadConfig()
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
	t.Setenv("S3_USE_SSL", "true")
	t.Setenv("S3_PATH_STYLE", "true")

	cfg := LoadConfig()

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
		{"S3.UseSSL", cfg.S3.UseSSL, true},
		{"S3.PathStyle", cfg.S3.PathStyle, true},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("want %v, got %v", c.want, c.got)
			}
		})
	}
}

func TestValidateS3RequiresEndpoint(t *testing.T) {
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
	cfg := &Config{
		StorageType: "local",
		JWTSecret:   "some-safe-secret",
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for local storage, got %v", err)
	}
}

func TestS3RegionDefault(t *testing.T) {
	cfg := LoadConfig()
	if cfg.S3.Region != "us-east-1" {
		t.Errorf("default S3 region: want %q, got %q", "us-east-1", cfg.S3.Region)
	}
}

func TestLoadConfigS3FromYAMLFile(t *testing.T) {
	t.Setenv("CONFIG_FILE", "testdata/full.yaml")
	cfg := LoadConfig()

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
		{"S3.UseSSL", cfg.S3.UseSSL, true},
		{"S3.PathStyle", cfg.S3.PathStyle, true},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("want %v, got %v", c.want, c.got)
			}
		})
	}
}
