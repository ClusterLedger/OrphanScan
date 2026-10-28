package awsauth

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_DefaultProfile(t *testing.T) {
	isolateAWSConfig(t)
	// LoadConfig with empty profile and region should not error; it uses the
	// SDK default credential chain which always returns a config.
	cfg, err := LoadConfig(context.Background(), "", "")
	if err != nil {
		t.Fatalf("LoadConfig with defaults: %v", err)
	}
	_ = cfg
}

func TestLoadConfig_WithRegion(t *testing.T) {
	isolateAWSConfig(t)
	cfg, err := LoadConfig(context.Background(), "", "us-west-2")
	if err != nil {
		t.Fatalf("LoadConfig with region: %v", err)
	}
	if cfg.Region != "us-west-2" {
		t.Errorf("region = %q, want us-west-2", cfg.Region)
	}
}

func TestLoadConfigRegionSources(t *testing.T) {
	for _, tt := range []struct {
		name, environment, explicit, want string
	}{
		{name: "profile region", want: "eu-west-1"},
		{name: "environment overrides profile", environment: "ap-south-1", want: "ap-south-1"},
		{name: "explicit overrides environment and profile", environment: "ap-south-1", explicit: "us-west-2", want: "us-west-2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			isolateAWSConfig(t)
			if err := os.WriteFile(os.Getenv("AWS_CONFIG_FILE"), []byte("[profile audit]\nregion = eu-west-1\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("AWS_REGION", tt.environment)
			cfg, err := LoadConfig(context.Background(), "audit", tt.explicit)
			if err != nil || cfg.Region != tt.want {
				t.Fatalf("region=%q want=%q err=%v", cfg.Region, tt.want, err)
			}
		})
	}
}

func isolateAWSConfig(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "AWS_") {
			t.Setenv(name, "")
		}
	}
	dir := t.TempDir()
	for _, name := range []string{"config", "credentials"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "credentials"))
	for _, name := range []string{"AWS_PROFILE", "AWS_DEFAULT_PROFILE", "AWS_REGION", "AWS_DEFAULT_REGION", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_CREDENTIALS_FULL_URI"} {
		t.Setenv(name, "")
	}
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}
