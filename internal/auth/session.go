package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Session cookie format (all base64-url-nopad, dot-separated):
//
//	v1.<user_id>.<expires_unix>.<hmac(secret, "v1."+user_id+"."+expires)>
//
// Self-contained: no server-side session table. Rotating auth.session_secret
// invalidates every outstanding cookie.
const (
	SessionCookieName    = "bindery_session"
	SessionDuration      = 30 * 24 * time.Hour // when "remember me" is checked
	SessionDurationShort = 12 * time.Hour      // browser-session equivalent when not
	sessionCookieVersion = "v1"
)

// SignSession returns a signed cookie value for the given user that expires at exp.
func SignSession(secret []byte, userID int64, exp time.Time) string {
	payload := fmt.Sprintf("%s.%d.%d", sessionCookieVersion, userID, exp.Unix())
	mac := hmacSum(secret, payload)
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac)
}

// VerifySession returns the user id carried by a valid, unexpired signed cookie,
// or an error on any tamper/expiry/parse failure.
func VerifySession(secret []byte, cookie string) (int64, error) {
	parts := strings.Split(cookie, ".")
	if len(parts) != 4 || parts[0] != sessionCookieVersion {
		return 0, errors.New("malformed session")
	}
	userID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, errors.New("bad user id")
	}
	expUnix, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0, errors.New("bad expiry")
	}
	payload := strings.Join(parts[:3], ".")
	want := hmacSum(secret, payload)
	got, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return 0, errors.New("bad signature encoding")
	}
	if !hmac.Equal(got, want) {
		return 0, errors.New("bad signature")
	}
	if time.Now().Unix() > expUnix {
		return 0, errors.New("expired")
	}
	return userID, nil
}

func hmacSum(secret []byte, payload string) []byte {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(payload))
	return h.Sum(nil)
}
