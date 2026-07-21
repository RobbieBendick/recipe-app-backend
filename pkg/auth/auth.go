package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/api/idtoken"
)

type contextKey string

const userIDKey contextKey = "userID"

type Service struct {
	JWTSecret      []byte
	GoogleClientID string
	TokenTTL       time.Duration
}

type Claims struct {
	UserID string `json:"uid"`
	jwt.RegisteredClaims
}

type GoogleProfile struct {
	Sub       string
	Email     string
	Name      string
	AvatarURL string
}

func NewService(jwtSecret, googleClientID string) (*Service, error) {
	secret := strings.TrimSpace(jwtSecret)
	if secret == "" {
		return nil, errors.New("JWT_SECRET is required")
	}
	return &Service{
		JWTSecret:      []byte(secret),
		GoogleClientID: strings.TrimSpace(googleClientID),
		TokenTTL:       30 * 24 * time.Hour,
	}, nil
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (s *Service) SignToken(userID string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.TokenTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.JWTSecret)
}

func (s *Service) ParseToken(tokenString string) (string, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.JWTSecret, nil
	})
	if err != nil {
		return "", err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid || claims.UserID == "" {
		return "", errors.New("invalid token")
	}
	return claims.UserID, nil
}

func (s *Service) VerifyGoogleIDToken(ctx context.Context, rawToken string) (*GoogleProfile, error) {
	if s.GoogleClientID == "" {
		return nil, errors.New("GOOGLE_CLIENT_ID is not configured")
	}
	payload, err := idtoken.Validate(ctx, rawToken, s.GoogleClientID)
	if err != nil {
		return nil, err
	}
	email, _ := payload.Claims["email"].(string)
	if email == "" {
		return nil, errors.New("google token missing email")
	}
	name, _ := payload.Claims["name"].(string)
	picture, _ := payload.Claims["picture"].(string)
	return &GoogleProfile{
		Sub:       payload.Subject,
		Email:     email,
		Name:      name,
		AvatarURL: picture,
	}, nil
}

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDKey).(string)
	return userID, ok && userID != ""
}

func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		userID, err := s.ParseToken(token)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), userID)))
	})
}

func BearerUserID(s *Service, r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return "", errors.New("missing bearer token")
	}
	return s.ParseToken(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
}
