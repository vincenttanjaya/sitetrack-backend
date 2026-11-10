package platform

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Addr         string
	DatabaseURL  string
	JWTSecret    []byte
	EphemeralJWT bool
	Location     *time.Location
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
}

func LoadConfig() (Config, error) {
	c := Config{
		Addr:        envDefault("APP_ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   []byte(os.Getenv("JWT_SECRET")),
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	if len(c.JWTSecret) == 0 {
		c.JWTSecret = make([]byte, 32)
		rand.Read(c.JWTSecret)
		c.EphemeralJWT = true
	} else if len(c.JWTSecret) < 32 {
		return c, errors.New("JWT_SECRET must be at least 32 bytes")
	}

	var err error
	zone := envDefault("BUSINESS_TIMEZONE", "Asia/Muscat")
	if zone != "UTC" && !strings.Contains(zone, "/") {
		return c, errors.New("BUSINESS_TIMEZONE must be UTC or a regional IANA name such as Asia/Muscat")
	}
	c.Location, err = time.LoadLocation(zone)
	if err != nil {
		return c, fmt.Errorf("BUSINESS_TIMEZONE: %w", err)
	}
	c.AccessTTL, err = positiveDuration("ACCESS_TOKEN_TTL", "15m")
	if err != nil {
		return c, err
	}
	c.RefreshTTL, err = positiveDuration("REFRESH_TOKEN_TTL", "720h")
	if err != nil {
		return c, err
	}
	if c.RefreshTTL <= c.AccessTTL {
		return c, errors.New("REFRESH_TOKEN_TTL must exceed ACCESS_TOKEN_TTL")
	}
	return c, nil
}

func envDefault(key, value string) string {
	if env := os.Getenv(key); env != "" {
		return env
	}
	return value
}

func positiveDuration(key, value string) (time.Duration, error) {
	d, err := time.ParseDuration(envDefault(key, value))
	if err != nil || d < time.Second {
		return 0, fmt.Errorf("%s must be a duration of at least one second", key)
	}
	return d, nil
}
