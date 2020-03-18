// Copyright 2020 Google LLC.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package idtoken

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"math/big"
	"net/http"
	"testing"

	"google.golang.org/api/option"
)

const (
	keyID        = "1234"
	testAudience = "test-audience"
)

func TestValidateRS256(t *testing.T) {
	idToken, pk := createRS256JWT(t)
	tests := []struct {
		name    string
		keyID   string
		n       *big.Int
		e       int
		wantErr bool
	}{
		{name: "works", keyID: keyID, n: pk.N, e: pk.E, wantErr: false},
		{name: "no matching key", keyID: "5678", n: pk.N, e: pk.E, wantErr: true},
		{name: "sig does not match", keyID: keyID, n: new(big.Int).SetBytes([]byte("42")), e: 42, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{
				Transport: RoundTripFn(func(req *http.Request) *http.Response {
					cr := certResponse{
						Keys: []jwk{
							{
								Kid: tt.keyID,
								N:   base64.RawURLEncoding.EncodeToString(tt.n.Bytes()),
								E:   base64.RawURLEncoding.EncodeToString(new(big.Int).SetInt64(int64(tt.e)).Bytes()),
							},
						},
					}
					b, err := json.Marshal(&cr)
					if err != nil {
						t.Fatalf("unable to marshal response: %v", err)
					}
					return &http.Response{
						StatusCode: 200,
						Body:       ioutil.NopCloser(bytes.NewReader(b)),
						Header:     make(http.Header),
					}
				}),
			}

			v, err := NewValidator(context.Background(), option.WithHTTPClient(client))
			if err != nil {
				t.Fatalf("NewValidator(...) = %q, want nil", err)
			}
			payload, err := v.Validate(context.Background(), idToken, testAudience)
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate(ctx, %s, %s) = %q, want nil", idToken, testAudience, err)
			}
			if !tt.wantErr && payload.Audience != testAudience {
				t.Fatalf("got %v, want %v", payload.Audience, testAudience)
			}
		})
	}
}

func TestValidateES256(t *testing.T) {
	idToken, pk := createES256JWT(t)
	tests := []struct {
		name    string
		keyID   string
		x       *big.Int
		y       *big.Int
		wantErr bool
	}{
		{name: "works", keyID: keyID, x: pk.X, y: pk.Y, wantErr: false},
		{name: "no matching key", keyID: "5678", x: pk.X, y: pk.Y, wantErr: true},
		{name: "sig does not match", keyID: keyID, x: new(big.Int), y: new(big.Int), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{
				Transport: RoundTripFn(func(req *http.Request) *http.Response {
					cr := certResponse{
						Keys: []jwk{
							{
								Kid: tt.keyID,
								X:   base64.RawURLEncoding.EncodeToString(tt.x.Bytes()),
								Y:   base64.RawURLEncoding.EncodeToString(tt.y.Bytes()),
							},
						},
					}
					b, err := json.Marshal(&cr)
					if err != nil {
						t.Fatalf("unable to marshal response: %v", err)
					}
					return &http.Response{
						StatusCode: 200,
						Body:       ioutil.NopCloser(bytes.NewReader(b)),
						Header:     make(http.Header),
					}
				}),
			}

			v, err := NewValidator(context.Background(), option.WithHTTPClient(client))
			if err != nil {
				t.Fatalf("NewValidator(...) = %q, want nil", err)
			}
			payload, err := v.Validate(context.Background(), idToken, testAudience)
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate(ctx, %s, %s) = %q, want nil", idToken, testAudience, err)
			}
			if !tt.wantErr && payload.Audience != testAudience {
				t.Fatalf("got %v, want %v", payload.Audience, testAudience)
			}
		})
	}
}

func createES256JWT(t *testing.T) (string, ecdsa.PublicKey) {
	t.Helper()
	token := commonToken(t, "ES256")
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("unable to generate key: %v", err)
	}
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, token.hashedContent())
	if err != nil {
		t.Fatalf("unable to sign content: %v", err)
	}
	var sig []byte
	sig = append(sig, r.Bytes()...)
	sig = append(sig, s.Bytes()...)
	token.signature = base64.RawURLEncoding.EncodeToString(sig)
	return token.String(), privateKey.PublicKey
}

func createRS256JWT(t *testing.T) (string, rsa.PublicKey) {
	t.Helper()
	token := commonToken(t, "RS256")
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("unable to generate key: %v", err)
	}
	sig, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, token.hashedContent())
	if err != nil {
		t.Fatalf("unable to sign content: %v", err)
	}
	token.signature = base64.RawURLEncoding.EncodeToString(sig)
	return token.String(), privateKey.PublicKey
}

func commonToken(t *testing.T, alg string) *jwt {
	t.Helper()
	header := jwtHeader{
		KeyID:     keyID,
		Algorithm: alg,
		Type:      "JWT",
	}
	payload := Payload{
		Issuer:   "example.com",
		Audience: testAudience,
	}

	hb, err := json.Marshal(&header)
	if err != nil {
		t.Fatalf("unable to marshall header: %v", err)
	}
	pb, err := json.Marshal(&payload)
	if err != nil {
		t.Fatalf("unable to marshall payload: %v", err)
	}
	eb := base64.RawURLEncoding.EncodeToString(hb)
	ep := base64.RawURLEncoding.EncodeToString(pb)
	return &jwt{
		header:  eb,
		payload: ep,
	}
}

type RoundTripFn func(req *http.Request) *http.Response

func (f RoundTripFn) RoundTrip(req *http.Request) (*http.Response, error) { return f(req), nil }
