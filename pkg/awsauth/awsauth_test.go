package awsauth

import (
	"context"
	"testing"
)

func TestLoadConfig_DefaultProfile(t *testing.T) {
	// LoadConfig with empty profile and region should not error; it uses the
	// SDK default credential chain which always returns a config.
	cfg, err := LoadConfig(context.Background(), "", "")
	if err != nil {
		t.Fatalf("LoadConfig with defaults: %v", err)
	}
	_ = cfg
}

func TestLoadConfig_WithRegion(t *testing.T) {
	cfg, err := LoadConfig(context.Background(), "", "us-west-2")
	if err != nil {
		t.Fatalf("LoadConfig with region: %v", err)
	}
	if cfg.Region != "us-west-2" {
		t.Errorf("region = %q, want us-west-2", cfg.Region)
	}
}
