package auth

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

type JWK struct {
	Kty string `json:"kty"`
	N   string `json:"n"`
	E   string `json:"e"`
	Kid string `json:"kid"`
}

type JWKS struct {
	Keys []JWK `json:"keys"`
}

// decode base64url
func b64Decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// convertir JWK → clé RSA
func jwkToPublicKey(jwk JWK) (*rsa.PublicKey, error) {
	nBytes, err := b64Decode(jwk.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := b64Decode(jwk.E)
	if err != nil {
		return nil, err
	}

	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)

	return &rsa.PublicKey{
		N: n,
		E: int(e.Int64()),
	}, nil
}

// parser JWT
func ParseJWT(token string, jwks JWKS) (map[string]interface{}, error) {

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token")
	}

	headerB, _ := b64Decode(parts[0])
	payloadB, _ := b64Decode(parts[1])
	signature, _ := b64Decode(parts[2])

	var header map[string]interface{}
	json.Unmarshal(headerB, &header)

	kid := header["kid"].(string)

	// trouver la clé
	var pubKey *rsa.PublicKey
	for _, k := range jwks.Keys {
		if k.Kid == kid {
			pubKey, _ = jwkToPublicKey(k)
		}
	}

	if pubKey == nil {
		return nil, fmt.Errorf("no key found")
	}

	// vérifier signature
	data := parts[0] + "." + parts[1]
	hash := sha256.Sum256([]byte(data))

	err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hash[:], signature)
	if err != nil {
		return nil, fmt.Errorf("invalid signature")
	}

	// lire claims
	var claims map[string]interface{}
	json.Unmarshal(payloadB, &claims)

	return claims, nil
}
