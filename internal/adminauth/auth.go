package adminauth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	AccessTTL  = 15 * time.Minute
	RefreshTTL = 7 * 24 * time.Hour
)

type Claims struct {
	jwt.RegisteredClaims
	Email   string `json:"email"`
	IsAdmin bool   `json:"is_admin"`
}

func secret() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("AIPROXY_JWT_SECRET"))
	if raw == "" {
		return nil, fmt.Errorf("set AIPROXY_JWT_SECRET")
	}
	return []byte(raw), nil
}

func IssueAccess(userID, email string, isAdmin bool) (string, time.Time, error) {
	key, err := secret()
	if err != nil {
		return "", time.Time{}, err
	}
	exp := time.Now().Add(AccessTTL)
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: userID, ExpiresAt: jwt.NewNumericDate(exp), IssuedAt: jwt.NewNumericDate(time.Now())},
		Email:            email,
		IsAdmin:          isAdmin,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(key)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

func VerifyAccess(tokenString string) (*Claims, error) {
	key, err := secret()
	if err != nil {
		return nil, err
	}
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return key, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

func NewRefreshToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
