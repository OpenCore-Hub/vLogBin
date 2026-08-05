// Package crypto provides AES-256-GCM encryption for sensitive credentials
// that must be decryptable at runtime (e.g. PSP API keys). The master key
// is provided via configuration and never stored in the database.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// Encryptor encrypts and decrypts strings using AES-256-GCM. It holds one
// active master key — used for every new encryption — plus zero or more
// previous master keys retained solely to decrypt ciphertext written before
// a key rotation. Decryption tries the active key first and then falls back
// to each previous key in order, so rotating the master key never orphans
// existing credentials.
type Encryptor struct {
	active   cipher.AEAD
	previous []cipher.AEAD
	// fallbackObserver, when set, is invoked every time Decrypt succeeds via
	// a previous (rotated-out) key. main wires it to a counter so operators
	// can see how much legacy ciphertext is still being read and schedule
	// re-encryption. May be called concurrently.
	fallbackObserver func()
}

// NewEncryptor creates an Encryptor from a 64-character hex string
// (32 bytes) with no previous keys. Returns an error if the key is invalid.
func NewEncryptor(masterKeyHex string) (*Encryptor, error) {
	return NewEncryptorWithPrevious(masterKeyHex, nil)
}

// NewEncryptorWithPrevious creates an Encryptor with an active master key
// and an optional list of previous master keys (each a 64-char hex string)
// that are kept only for decrypting pre-rotation ciphertext. New ciphertext
// is always sealed with the active key. Every previous key must be a valid
// 32-byte key — a typo here would silently orphan all ciphertext written
// under that key.
func NewEncryptorWithPrevious(masterKeyHex string, previousKeys []string) (*Encryptor, error) {
	active, err := newAEAD(masterKeyHex)
	if err != nil {
		return nil, err
	}
	e := &Encryptor{active: active}
	for i, k := range previousKeys {
		aead, err := newAEAD(k)
		if err != nil {
			return nil, fmt.Errorf("previous key %d: %w", i, err)
		}
		e.previous = append(e.previous, aead)
	}
	return e, nil
}

// SetFallbackObserver installs a callback invoked each time Decrypt succeeds
// using a previous (rotated-out) key. Passing nil disables it.
func (e *Encryptor) SetFallbackObserver(fn func()) {
	e.fallbackObserver = fn
}

func newAEAD(keyHex string) (cipher.AEAD, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("decode master key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("master key must be 32 bytes (64 hex chars), got %d bytes", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	return aead, nil
}

// Encrypt encrypts plaintext and returns a base64-encoded ciphertext
// containing the nonce prepended to the sealed data. The active master key
// is always used, so freshly written credentials are readable with the
// active key alone.
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, e.active.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := e.active.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt decodes a base64 ciphertext and returns the plaintext. The active
// key is tried first; if authentication fails, each previous key is tried in
// order so ciphertext written before a master-key rotation stays readable.
// When a previous key succeeds, the fallback observer (if any) is invoked.
func (e *Encryptor) Decrypt(ciphertext string) (string, error) {
	return e.decrypt(ciphertext, true)
}

// DecryptWithoutFallback is Decrypt minus the fallback-observer notification.
// The credential re-encryption worker uses it so credential_decrypt_fallback_total
// keeps its operational meaning — "legacy ciphertext still being read on the
// request path" — while worker-driven re-encryptions are counted separately
// by credentials_reencrypted_total.
func (e *Encryptor) DecryptWithoutFallback(ciphertext string) (string, error) {
	return e.decrypt(ciphertext, false)
}

func (e *Encryptor) decrypt(ciphertext string, notify bool) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	ns := e.active.NonceSize()
	if len(data) < ns {
		return "", errors.New("ciphertext too short")
	}
	nonce, sealed := data[:ns], data[ns:]
	plaintext, err := e.active.Open(nil, nonce, sealed, nil)
	if err == nil {
		return string(plaintext), nil
	}
	for _, aead := range e.previous {
		pt, perr := aead.Open(nil, nonce, sealed, nil)
		if perr == nil {
			if notify && e.fallbackObserver != nil {
				e.fallbackObserver()
			}
			return string(pt), nil
		}
	}
	if len(e.previous) == 0 {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return "", fmt.Errorf("decrypt: active key and %d previous key(s) failed", len(e.previous))
}

// NeedsReencryption reports whether ciphertext was sealed with a previous
// (rotated-out) master key and should be re-encrypted with the active key to
// converge after a master-key rotation. It is detection only: no plaintext is
// exposed and the fallback observer is not fired. An error is returned when
// no configured key can open the ciphertext (corrupt or sealed under an
// unknown key); the re-encryption worker then skips the row and counts it as
// unrecoverable.
func (e *Encryptor) NeedsReencryption(ciphertext string) (bool, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return false, fmt.Errorf("decode ciphertext: %w", err)
	}
	ns := e.active.NonceSize()
	if len(data) < ns {
		return false, errors.New("ciphertext too short")
	}
	nonce, sealed := data[:ns], data[ns:]
	if _, err := e.active.Open(nil, nonce, sealed, nil); err == nil {
		return false, nil
	}
	for _, aead := range e.previous {
		if _, perr := aead.Open(nil, nonce, sealed, nil); perr == nil {
			return true, nil
		}
	}
	if len(e.previous) == 0 {
		return false, fmt.Errorf("open: active key failed")
	}
	return false, fmt.Errorf("open: active key and %d previous key(s) failed", len(e.previous))
}
