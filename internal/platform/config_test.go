package platform_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/vincenttanjaya/sitetrack-backend/internal/platform"
)

func TestLocalRuntimeGetsIndependentSigningKeys(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://local/sitetrack")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("APP_ADDR", "")
	t.Setenv("BUSINESS_TIMEZONE", "")
	t.Setenv("ACCESS_TOKEN_TTL", "")
	t.Setenv("REFRESH_TOKEN_TTL", "")
	first, err := platform.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	second, err := platform.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(first.JWTSecret) != 32 || len(second.JWTSecret) != 32 || bytes.Equal(first.JWTSecret, second.JWTSecret) {
		t.Fatal("expected independent 32-byte process signing keys")
	}
	if first.Addr != ":8080" || first.Location.String() != "Asia/Muscat" || first.AccessTTL != 15*time.Minute || first.RefreshTTL != 30*24*time.Hour {
		t.Fatalf("unexpected runtime defaults: %s %s %v %v", first.Addr, first.Location, first.AccessTTL, first.RefreshTTL)
	}
}

func TestRuntimeRejectsInvalidAuthConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://local/sitetrack")
	t.Setenv("JWT_SECRET", "short")
	if _, err := platform.LoadConfig(); err == nil {
		t.Fatal("accepted a short signing secret")
	}
	t.Setenv("JWT_SECRET", "an explicit test secret at least 32 bytes")
	t.Setenv("ACCESS_TOKEN_TTL", "60m")
	t.Setenv("REFRESH_TOKEN_TTL", "30m")
	if _, err := platform.LoadConfig(); err == nil {
		t.Fatal("accepted refresh duration shorter than access")
	}
}

func TestRuntimeRequiresUnambiguousBusinessTimezone(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://local/sitetrack")
	t.Setenv("JWT_SECRET", "")
	for _, zone := range []string{"Local", "CET", "EET", "WET", "MET"} {
		t.Setenv("BUSINESS_TIMEZONE", zone)
		if _, err := platform.LoadConfig(); err == nil {
			t.Errorf("accepted ambiguous timezone %s", zone)
		}
	}
	for _, zone := range []string{"UTC", "Europe/Paris", "Etc/GMT+4"} {
		t.Setenv("BUSINESS_TIMEZONE", zone)
		if cfg, err := platform.LoadConfig(); err != nil || cfg.Location.String() != zone {
			t.Errorf("failed to load timezone %s: %v", zone, err)
		}
	}
}
