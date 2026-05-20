package job

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const AlgorithmHMACSHA256 = "HMAC-SHA256"

// Sign returns a signed job envelope using HMAC-SHA256.
func Sign(job Job, keyID string, secret []byte) (SignedJob, error) {
	if err := job.Validate(); err != nil {
		return SignedJob{}, err
	}
	keyID = strings.TrimSpace(keyID)
	if err := validateToken(keyID); err != nil {
		return SignedJob{}, fmt.Errorf("keyID: %w", err)
	}
	if len(secret) < 32 {
		return SignedJob{}, errors.New("secret must be at least 32 bytes")
	}
	envelope := SignedJob{
		Algorithm: AlgorithmHMACSHA256,
		KeyID:     keyID,
		Job:       job.Clone(),
	}
	signature, err := signPayload(envelope, secret)
	if err != nil {
		return SignedJob{}, err
	}
	envelope.Signature = signature
	return envelope, nil
}

// Verify validates the envelope signature, expiry, and job payload.
func Verify(envelope SignedJob, keys KeyResolver, now time.Time) (Job, error) {
	if envelope.Algorithm != AlgorithmHMACSHA256 {
		return Job{}, fmt.Errorf("unsupported signing algorithm %q", envelope.Algorithm)
	}
	if err := validateToken(envelope.KeyID); err != nil {
		return Job{}, fmt.Errorf("keyId: %w", err)
	}
	if strings.TrimSpace(envelope.Signature) == "" {
		return Job{}, errors.New("signature is required")
	}
	if keys == nil {
		return Job{}, errors.New("key resolver is required")
	}
	if err := envelope.Job.Validate(); err != nil {
		return Job{}, err
	}
	if envelope.Job.Expired(now) {
		return Job{}, errors.New("job has expired")
	}
	secret, err := keys.ResolveKey(envelope.KeyID)
	if err != nil {
		return Job{}, fmt.Errorf("resolve key %q: %w", envelope.KeyID, err)
	}
	if len(secret) < 32 {
		return Job{}, errors.New("resolved secret must be at least 32 bytes")
	}
	expected, err := signPayload(SignedJob{
		Algorithm: envelope.Algorithm,
		KeyID:     envelope.KeyID,
		Job:       envelope.Job,
	}, secret)
	if err != nil {
		return Job{}, err
	}
	if subtle.ConstantTimeCompare([]byte(expected), []byte(envelope.Signature)) != 1 {
		return Job{}, errors.New("invalid job signature")
	}
	return envelope.Job.Clone(), nil
}

func signPayload(envelope SignedJob, secret []byte) (string, error) {
	payload := struct {
		Algorithm string `json:"algorithm"`
		KeyID     string `json:"keyId"`
		Job       Job    `json:"job"`
	}{
		Algorithm: envelope.Algorithm,
		KeyID:     envelope.KeyID,
		Job:       envelope.Job.Clone(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal signed payload: %w", err)
	}
	mac := hmac.New(sha256.New, secret)
	if _, err := mac.Write(raw); err != nil {
		return "", fmt.Errorf("sign payload: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
