package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"github.com/vincenttanjaya/sitetrack-backend/internal/ports"
	"golang.org/x/crypto/bcrypt"
)

const (
	tokenIssuer       = "sitetrack"
	tokenAudience     = "sitetrack-api"
	dummyPasswordHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
)

type AuthService struct {
	repo                  ports.AuthRepository
	signingKey            []byte
	clock                 func() time.Time
	accessTTL, refreshTTL time.Duration
}

func NewAuth(repo ports.AuthRepository, key []byte, clock func() time.Time, accessTTL, refreshTTL time.Duration) *AuthService {
	return &AuthService{repo: repo, signingKey: key, clock: clock, accessTTL: accessTTL, refreshTTL: refreshTTL}
}

func (s *AuthService) Register(ctx context.Context, in domain.RegisterInput) (domain.User, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Email = normalizedEmail(in.Email)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 150 || strings.ContainsRune(in.Name, 0) {
		return domain.User{}, domain.Invalid("name must be between 1 and 150 characters")
	}
	if !in.Role.Valid() {
		return domain.User{}, domain.Invalid("unknown app role")
	}
	address, err := mail.ParseAddress(in.Email)
	if err != nil || address.Address != in.Email || len(in.Email) > 254 {
		return domain.User{}, domain.Invalid("email is invalid")
	}
	for _, c := range in.Email {
		if c > 127 {
			return domain.User{}, domain.Invalid("email must use ASCII characters")
		}
	}
	if len(in.Password) < 8 || len(in.Password) > 72 {
		return domain.User{}, domain.Invalid("password must be between 8 and 72 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, err
	}
	return s.repo.CreateUser(ctx, in, string(hash))
}

func normalizedEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func (s *AuthService) Login(ctx context.Context, email, password string) (domain.TokenPair, error) {
	user, hash, err := s.repo.Credentials(ctx, normalizedEmail(email))
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return domain.TokenPair{}, err
	}
	missing := errors.Is(err, domain.ErrNotFound)
	if missing {
		hash = dummyPasswordHash
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil || missing {
		return domain.TokenPair{}, domain.ErrUnauthorized
	}
	now := s.clock()
	refresh, digest := newRefreshToken()
	session := domain.Session{ID: rand.Text(), UserID: user.ID, RefreshHash: digest, ExpiresAt: now.Add(s.refreshTTL)}
	tokens, err := s.issue(session, refresh, now)
	if err != nil {
		return domain.TokenPair{}, err
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return domain.TokenPair{}, err
	}
	return tokens, nil
}

func newRefreshToken() (string, [32]byte) {
	var token [32]byte
	rand.Read(token[:])
	return base64.RawURLEncoding.EncodeToString(token[:]), sha256.Sum256(token[:])
}

func (s *AuthService) Refresh(ctx context.Context, previous string) (domain.TokenPair, error) {
	if len(previous) != 43 {
		return domain.TokenPair{}, domain.ErrUnauthorized
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(previous)
	if err != nil || len(decoded) != 32 {
		return domain.TokenPair{}, domain.ErrUnauthorized
	}
	token, digest := newRefreshToken()
	now := s.clock()
	session, err := s.repo.RotateSession(ctx, sha256.Sum256(decoded), digest, now, now.Add(s.refreshTTL))
	if errors.Is(err, domain.ErrNotFound) {
		return domain.TokenPair{}, domain.ErrUnauthorized
	}
	if err != nil {
		return domain.TokenPair{}, err
	}
	return s.issue(session, token, now)
}

type accessClaims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

func (s *AuthService) issue(session domain.Session, refresh string, now time.Time) (domain.TokenPair, error) {
	expiry := now.Add(s.accessTTL)
	if session.ExpiresAt.Before(expiry) {
		expiry = session.ExpiresAt
	}
	claims := accessClaims{SessionID: session.ID, RegisteredClaims: jwt.RegisteredClaims{
		Issuer: tokenIssuer, Audience: jwt.ClaimStrings{tokenAudience}, Subject: strconv.FormatInt(session.UserID, 10),
		ID: rand.Text(), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expiry),
	}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.signingKey)
	if err != nil {
		return domain.TokenPair{}, err
	}
	return domain.TokenPair{AccessToken: token, RefreshToken: refresh, ExpiresIn: int64(expiry.Sub(now) / time.Second)}, nil
}

func (s *AuthService) Authenticate(ctx context.Context, access string) (domain.Actor, error) {
	var claims accessClaims
	if len(access) > 4096 {
		return domain.Actor{}, domain.ErrUnauthorized
	}
	now := s.clock()
	_, err := jwt.ParseWithClaims(access, &claims, func(*jwt.Token) (any, error) { return s.signingKey, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuedAt(),
		jwt.WithIssuer(tokenIssuer), jwt.WithAudience(tokenAudience), jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil || claims.SessionID == "" || claims.IssuedAt == nil {
		return domain.Actor{}, domain.ErrUnauthorized
	}
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return domain.Actor{}, domain.ErrUnauthorized
	}
	actor, err := s.repo.CurrentActor(ctx, claims.SessionID, userID, now)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Actor{}, domain.ErrUnauthorized
	}
	return actor, err
}

func (s *AuthService) Profile(ctx context.Context, actor domain.Actor) (domain.User, error) {
	return s.repo.User(ctx, actor.UserID)
}

func (s *AuthService) Logout(ctx context.Context, actor domain.Actor) error {
	return s.repo.RevokeSession(ctx, actor, s.clock())
}
