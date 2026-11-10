package auth

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/scrypt"
)

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), 11)
	return string(b), err
}

// VerifyPassword checks pw against the current bcrypt hash, or, when the
// account was imported, against the legacy hash in its original format.
// upgrade is true when the caller should store a fresh bcrypt hash.
func VerifyPassword(hash, legacy, pw string) (ok, upgrade bool) {
	if hash != "" {
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil, false
	}
	if legacy != "" && verifyLegacy(legacy, pw) {
		return true, true
	}
	return false, false
}

// Legacy formats:
//   $2a$ / $2b$                    guarantee system (bcrypt) - same algorithm, usable as is
//   scrypt$<salt hex>$<hash hex>   warehouse system (N=16384 r=8 p=1, 64 bytes)
//   pbkdf2_sha256$<iter>$<salt>$<hex>  attendance system (salt used as raw text)
func verifyLegacy(stored, pw string) bool {
	switch {
	case strings.HasPrefix(stored, "$2"):
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(pw)) == nil
	case strings.HasPrefix(stored, "scrypt$"):
		parts := strings.Split(stored, "$")
		if len(parts) != 3 {
			return false
		}
		salt, err1 := hex.DecodeString(parts[1])
		want, err2 := hex.DecodeString(parts[2])
		if err1 != nil || err2 != nil || len(want) == 0 {
			return false
		}
		got, err := scrypt.Key([]byte(pw), salt, 16384, 8, 1, len(want))
		return err == nil && subtle.ConstantTimeCompare(got, want) == 1
	case strings.HasPrefix(stored, "pbkdf2_sha256$"):
		parts := strings.Split(stored, "$")
		if len(parts) != 4 {
			return false
		}
		iter, err := strconv.Atoi(parts[1])
		if err != nil || iter <= 0 {
			return false
		}
		want, err := hex.DecodeString(parts[3])
		if err != nil {
			return false
		}
		got, err := pbkdf2.Key(sha256.New, pw, []byte(parts[2]), iter, len(want))
		return err == nil && hmac.Equal(got, want)
	}
	return false
}

// IsBcrypt tells the importer whether a foreign hash can go straight into password_hash.
func IsBcrypt(h string) bool { return strings.HasPrefix(h, "$2") }
