package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/vincenttanjaya/sitetrack-backend/internal/adapters/postgres"
	"github.com/vincenttanjaya/sitetrack-backend/internal/application"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"github.com/vincenttanjaya/sitetrack-backend/internal/testdb"
)

func TestSessionsUseCurrentRoleAndRevokeOnLogout(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	now := time.Now().UTC().Truncate(time.Second)
	clock := func() time.Time { return now }
	secret := []byte("a test signing key of at least 32 bytes")
	auth := application.NewAuth(postgres.NewAuthRepository(db), secret, clock, time.Minute, time.Hour)
	user, err := auth.Register(ctx, domain.RegisterInput{Name: "Mara", Email: "  Mara@Example.com ", Password: "strong-password", Role: domain.RoleSuperAdmin})
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "mara@example.com" || user.Role != domain.RoleSuperAdmin {
		t.Fatalf("registered user = %+v", user)
	}
	pair, err := auth.Login(ctx, "MARA@example.com", "strong-password")
	if err != nil {
		t.Fatal(err)
	}
	other, err := auth.Login(ctx, user.Email, "strong-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE users SET role = ? WHERE id = ?", domain.RoleUser, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authenticate(ctx, pair.AccessToken)
	if err != nil || actor.Role != domain.RoleUser {
		t.Fatalf("current actor = %+v, %v", actor, err)
	}
	renewed, err := auth.Refresh(ctx, pair.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Refresh(ctx, pair.RefreshToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("reused refresh = %v", err)
	}
	if err := auth.Logout(ctx, actor); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{pair.AccessToken, renewed.AccessToken} {
		if _, err := auth.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("revoked access = %v", err)
		}
	}
	if _, err := auth.Refresh(ctx, renewed.RefreshToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("revoked refresh = %v", err)
	}
	if _, err := auth.Authenticate(ctx, other.AccessToken); err != nil {
		t.Fatalf("separate session = %v", err)
	}
	now = now.Add(time.Minute)
	if _, err := auth.Authenticate(ctx, other.AccessToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expired access = %v", err)
	}
	if _, err := auth.Refresh(ctx, other.RefreshToken); err != nil {
		t.Fatalf("active refresh after access expiry = %v", err)
	}
}

func TestInvalidCredentialsAndJWTAlgorithm(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	now := time.Now().UTC().Truncate(time.Second)
	secret := []byte("a test signing key of at least 32 bytes")
	auth := application.NewAuth(postgres.NewAuthRepository(db), secret, func() time.Time { return now }, time.Minute, time.Hour)
	user, err := auth.Register(ctx, domain.RegisterInput{Name: "Mara", Email: "mara@example.com", Password: "strong-password", Role: domain.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	for _, email := range []string{user.Email, "missing@example.com"} {
		if _, err := auth.Login(ctx, email, "wrong-password"); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("bad credentials = %v", err)
		}
	}
	pair, err := auth.Login(ctx, user.Email, "strong-password")
	if err != nil {
		t.Fatal(err)
	}
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(pair.AccessToken, claims); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		method   jwt.SigningMethod
		issuer   string
		audience string
	}{
		{"algorithm", jwt.SigningMethodHS384, "sitetrack", "sitetrack-api"},
		{"issuer", jwt.SigningMethodHS256, "elsewhere", "sitetrack-api"},
		{"audience", jwt.SigningMethodHS256, "sitetrack", "elsewhere"},
	} {
		t.Run(test.name, func(t *testing.T) {
			claims["iss"] = test.issuer
			claims["aud"] = test.audience
			forged, err := jwt.NewWithClaims(test.method, claims).SignedString(secret)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := auth.Authenticate(ctx, forged); !errors.Is(err, domain.ErrUnauthorized) {
				t.Fatalf("invalid JWT accepted: %v", err)
			}
		})
	}
}
