package auth

import "testing"

// Vectors produced by the real legacy implementations (Node scrypt in the
// warehouse system, Python pbkdf2 in the attendance system).
const (
	legacyScrypt = "scrypt$9f081b92472b287f21d41e37959d918b$85c94d1f2e0f0fe52c4c8aad994147e56fe3976a03fd0bc2a242f4bcaf9267b3357e13e895dbc4f4e7cddc9663c935d1bc8d580dbab8f041b9431fe833a32b75"
	legacyPBKDF2 = "pbkdf2_sha256$260000$abcdef0123456789$244e5a1a9a03975b6e5a12e5f56157c6b7983ab481b6a895d5bcae2f2abb4f9f"
)

func TestLegacyHashes(t *testing.T) {
	if ok, up := VerifyPassword("", legacyScrypt, "S3cret-pw"); !ok || !up {
		t.Fatalf("scrypt hash from the warehouse system must verify and request an upgrade")
	}
	if ok, _ := VerifyPassword("", legacyScrypt, "wrong"); ok {
		t.Fatalf("wrong password accepted (scrypt)")
	}
	if ok, up := VerifyPassword("", legacyPBKDF2, "4321"); !ok || !up {
		t.Fatalf("pbkdf2 hash from the attendance system must verify and request an upgrade")
	}
	if ok, _ := VerifyPassword("", legacyPBKDF2, "0000"); ok {
		t.Fatalf("wrong password accepted (pbkdf2)")
	}
}

func TestBcryptRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if ok, up := VerifyPassword(h, "", "correct horse battery"); !ok || up {
		t.Fatalf("bcrypt round trip failed")
	}
	if ok, _ := VerifyPassword(h, "", "nope"); ok {
		t.Fatalf("wrong password accepted")
	}
}
