package httpapi

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestVerifyAuthVaultJWT(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})

	server := &Server{authVaultPublicKey: string(publicPEM), authVaultAudience: "vlogbin-auth-vault"}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.RegisteredClaims{
		Issuer:    "vlogbin-web",
		Subject:   "web-backend",
		Audience:  jwt.ClaimStrings{"vlogbin-auth-vault"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})
	raw, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	if err := server.verifyAuthVaultJWT(raw); err != nil {
		t.Fatalf("verify valid token: %v", err)
	}

	wrong := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.RegisteredClaims{
		Issuer:    "vlogbin-web",
		Subject:   "web-backend",
		Audience:  jwt.ClaimStrings{"wrong-audience"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})
	wrongRaw, _ := wrong.SignedString(privateKey)
	if err := server.verifyAuthVaultJWT(wrongRaw); err == nil {
		t.Fatal("verify wrong audience token succeeded")
	}
}
