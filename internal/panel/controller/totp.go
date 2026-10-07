package controller

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func newTOTPSecret() (string, error) {
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret), nil
}

func generateTOTPRecoveryCodes() ([]string, []string, error) {
	codes := make([]string, 0, 10)
	hashes := make([]string, 0, 10)
	for range 10 {
		secret := make([]byte, 10)
		if _, err := rand.Read(secret); err != nil {
			return nil, nil, err
		}
		raw := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
		code := strings.Join([]string{raw[:4], raw[4:8], raw[8:12], raw[12:16]}, "-")
		codes = append(codes, code)
		hashes = append(hashes, recoveryCodeHash(code))
	}
	return codes, hashes, nil
}

func recoveryCodeHash(code string) string {
	normalized := strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, strings.ToUpper(strings.TrimSpace(code)))
	if len(normalized) != 16 {
		return ""
	}
	for _, r := range normalized {
		if (r < 'A' || r > 'Z') && (r < '2' || r > '7') {
			return ""
		}
	}
	digest := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(digest[:])
}

func verifyTOTP(secret, code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	if _, err := strconv.Atoi(code); err != nil {
		return false
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return false
	}
	for drift := int64(-1); drift <= 1; drift++ {
		counter := uint64(now.Unix()/30 + drift)
		var msg [8]byte
		binary.BigEndian.PutUint64(msg[:], counter)
		mac := hmac.New(sha1.New, key)
		_, _ = mac.Write(msg[:])
		sum := mac.Sum(nil)
		offset := sum[len(sum)-1] & 0x0f
		value := (uint32(sum[offset])&0x7f)<<24 | uint32(sum[offset+1])<<16 | uint32(sum[offset+2])<<8 | uint32(sum[offset+3])
		want := fmt.Sprintf("%06d", value%1_000_000)
		if subtle.ConstantTimeCompare([]byte(code), []byte(want)) == 1 {
			return true
		}
	}
	return false
}

func totpURL(email, secret string) string {
	label := url.PathEscape("Control Plane:" + email)
	query := url.Values{}
	query.Set("secret", secret)
	query.Set("issuer", "Control Plane")
	query.Set("algorithm", "SHA1")
	query.Set("digits", "6")
	query.Set("period", "30")
	return fmt.Sprintf("otpauth://totp/%s?%s", label, query.Encode())
}
