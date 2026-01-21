package core

import (
	"strings"
	"testing"
)

func TestHashToken(t *testing.T) {
	token := "eph_abc12345_secretsecretsecretsecret"

	hash, err := HashToken(token)
	if err != nil {
		t.Fatalf("HashToken() error = %v", err)
	}

	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("HashToken() hash should start with $argon2id$, got %s", hash)
	}

	hash2, err := HashToken(token)
	if err != nil {
		t.Fatalf("HashToken() second call error = %v", err)
	}

	if hash == hash2 {
		t.Error("HashToken() should produce different hashes due to random salt")
	}
}

func TestVerifyToken(t *testing.T) {
	token := "eph_abc12345_secretsecretsecretsecret"

	hash, err := HashToken(token)
	if err != nil {
		t.Fatalf("HashToken() error = %v", err)
	}

	if err := VerifyToken(token, hash); err != nil {
		t.Errorf("VerifyToken() with correct token error = %v", err)
	}

	if err := VerifyToken("wrong-token", hash); err != ErrHashMismatch {
		t.Errorf("VerifyToken() with wrong token error = %v, want ErrHashMismatch", err)
	}
}

func TestVerifyToken_InvalidHash(t *testing.T) {
	tests := []struct {
		name string
		hash string
	}{
		{"empty", ""},
		{"not argon2id", "$bcrypt$invalid"},
		{"missing parts", "$argon2id$v=19$m=65536"},
		{"invalid base64", "$argon2id$v=19$m=65536,t=1,p=4$!!!invalid!!!$!!!invalid!!!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := VerifyToken("any-token", tt.hash); err != ErrInvalidHash {
				t.Errorf("VerifyToken() error = %v, want ErrInvalidHash", err)
			}
		})
	}
}

func TestGenerateTokenSecret(t *testing.T) {
	secret1, err := GenerateTokenSecret(24)
	if err != nil {
		t.Fatalf("GenerateTokenSecret() error = %v", err)
	}

	if len(secret1) != 24 {
		t.Errorf("GenerateTokenSecret() length = %d, want 24", len(secret1))
	}

	secret2, err := GenerateTokenSecret(24)
	if err != nil {
		t.Fatalf("GenerateTokenSecret() second call error = %v", err)
	}

	if secret1 == secret2 {
		t.Error("GenerateTokenSecret() should produce different secrets")
	}
}

func TestBuildToken(t *testing.T) {
	token := BuildToken("abc12345", "secretsecretsecretsecret")
	expected := "eph_abc12345_secretsecretsecretsecret"

	if token != expected {
		t.Errorf("BuildToken() = %s, want %s", token, expected)
	}
}

func TestParseToken(t *testing.T) {
	t.Run("valid token", func(t *testing.T) {
		lookup, secret, err := ParseToken("eph_abc12345_secretsecretsecretsecret")
		if err != nil {
			t.Fatalf("ParseToken() error = %v", err)
		}

		if lookup != "abc12345" {
			t.Errorf("ParseToken() lookup = %s, want abc12345", lookup)
		}
		if secret != "secretsecretsecretsecret" {
			t.Errorf("ParseToken() secret = %s, want secretsecretsecretsecret", secret)
		}
	})

	t.Run("invalid prefix", func(t *testing.T) {
		_, _, err := ParseToken("xyz_abc_secret")
		if err != ErrInvalidToken {
			t.Errorf("ParseToken() error = %v, want ErrInvalidToken", err)
		}
	})

	t.Run("wrong number of parts", func(t *testing.T) {
		_, _, err := ParseToken("eph_abc")
		if err != ErrInvalidToken {
			t.Errorf("ParseToken() error = %v, want ErrInvalidToken", err)
		}
	})
}
