package auth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMintAndVerifyToken(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	token, err := MintToken("a-secret", now, time.Hour)
	if err != nil {
		t.Fatalf("MintToken returned error: %v", err)
	}

	expiry, err := VerifyToken("a-secret", token, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("VerifyToken rejected a freshly minted token: %v", err)
	}
	if want := now.Add(time.Hour).Unix(); expiry.Unix() != want {
		t.Errorf("expected expiry %d, got %d", want, expiry.Unix())
	}
}

func TestVerifyTokenRejections(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	valid, err := MintToken("a-secret", now, time.Hour)
	if err != nil {
		t.Fatalf("MintToken returned error: %v", err)
	}

	// A token signed with a different secret, used to prove that rotating the
	// secret invalidates outstanding sessions.
	otherSecret, err := MintToken("other-secret", now, time.Hour)
	if err != nil {
		t.Fatalf("MintToken returned error: %v", err)
	}

	tests := []struct {
		name    string
		secret  string
		token   string
		now     time.Time
		wantErr func(error) bool
	}{
		{
			name:    "expired token",
			secret:  "a-secret",
			token:   valid,
			now:     now.Add(2 * time.Hour),
			wantErr: IsErrTokenExpired,
		},
		{
			name:    "exactly at expiry",
			secret:  "a-secret",
			token:   valid,
			now:     now.Add(time.Hour),
			wantErr: IsErrTokenExpired,
		},
		{
			name:    "token minted with a different secret",
			secret:  "a-secret",
			token:   otherSecret,
			now:     now,
			wantErr: IsErrTokenInvalid,
		},
		{
			name:    "empty token",
			secret:  "a-secret",
			token:   "",
			now:     now,
			wantErr: IsErrTokenInvalid,
		},
		{
			name:    "not a jwt",
			secret:  "a-secret",
			token:   "garbage",
			now:     now,
			wantErr: IsErrTokenInvalid,
		},
		{
			name:    "missing signature segment",
			secret:  "a-secret",
			token:   strings.Join(strings.Split(valid, ".")[:2], "."),
			now:     now,
			wantErr: IsErrTokenInvalid,
		},
		{
			name:    "tampered signature",
			secret:  "a-secret",
			token:   valid[:len(valid)-1] + "X",
			now:     now,
			wantErr: IsErrTokenInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := VerifyToken(tt.secret, tt.token, tt.now)
			if err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !tt.wantErr(err) {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestVerifyTokenRejectsTamperedClaims covers the case that matters most: a
// caller extending their own expiry. The signature covers the claims, so any
// edit invalidates it.
func TestVerifyTokenRejectsTamperedClaims(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	token, err := MintToken("a-secret", now, time.Minute)
	if err != nil {
		t.Fatalf("MintToken returned error: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 token segments, got %d", len(parts))
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("failed to decode claims: %v", err)
	}
	var claims tokenClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		t.Fatalf("failed to unmarshal claims: %v", err)
	}
	claims.Exp = now.Add(100 * time.Hour).Unix()
	forged, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("failed to marshal forged claims: %v", err)
	}

	tampered := parts[0] + "." + base64.RawURLEncoding.EncodeToString(forged) + "." + parts[2]
	if _, err := VerifyToken("a-secret", tampered, now); !IsErrTokenInvalid(err) {
		t.Errorf("expected an invalid-token error for tampered claims, got %v", err)
	}
}

// TestVerifyTokenRejectsAlgNone guards the classic JWT footgun: a token that
// declares no signature algorithm must never be accepted.
func TestVerifyTokenRejectsAlgNone(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	claims := base64.RawURLEncoding.EncodeToString(
		[]byte(`{"sub":"rotom-ui","iat":1700000000,"exp":9999999999}`),
	)

	for _, signature := range []string{"", "anything"} {
		token := header + "." + claims + "." + signature
		if _, err := VerifyToken("a-secret", token, time.Unix(1_700_000_000, 0)); !IsErrTokenInvalid(err) {
			t.Errorf("expected an invalid-token error for alg=none, got %v", err)
		}
	}
}

// TestSigningKeyIsDerived asserts the signing key is not the raw secret, so a
// leaked signature can never be replayed as the secret itself.
func TestSigningKeyIsDerived(t *testing.T) {
	if got := string(signingKey("a-secret")); got == "a-secret" {
		t.Error("signing key must not equal the raw secret")
	}
	if a, b := signingKey("secret-one"), signingKey("secret-two"); string(a) == string(b) {
		t.Error("different secrets must derive different signing keys")
	}
}
