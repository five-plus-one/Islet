package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var ErrSecretUndecryptable = errors.New("已保存的密文无法用当前主密钥解密，请重新填写密钥后保存")

// SecretBox 管理主密钥（{dataDir}/islet.key，32 字节随机，0600）与 AES-256-GCM 密封。
type SecretBox struct {
	path string
	key  []byte
	once sync.Once
	err  error
}

func NewSecretBox(dataDir string) *SecretBox {
	return &SecretBox{path: filepath.Join(dataDir, "islet.key")}
}

func (b *SecretBox) load() error {
	b.once.Do(func() {
		if err := os.MkdirAll(filepath.Dir(b.path), 0700); err != nil {
			b.err = err
			return
		}
		raw, err := os.ReadFile(b.path)
		if err == nil {
			if key, decodeErr := hex.DecodeString(string(raw)); decodeErr == nil && len(key) == 32 {
				b.key = key
				return
			}
			// 文件损坏则重建
		} else if !os.IsNotExist(err) {
			b.err = err
			return
		}
		key := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, key); err != nil {
			b.err = err
			return
		}
		if err := os.WriteFile(b.path, []byte(hex.EncodeToString(key)), 0600); err != nil {
			b.err = err
			return
		}
		b.key = key
	})
	return b.err
}

func (b *SecretBox) Seal(plaintext string) (string, error) {
	if err := b.load(); err != nil {
		return "", err
	}
	block, err := aes.NewCipher(b.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plaintext), nil)), nil
}

func (b *SecretBox) Open(sealed string) (string, error) {
	if err := b.load(); err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return "", ErrSecretUndecryptable
	}
	block, err := aes.NewCipher(b.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", ErrSecretUndecryptable
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", ErrSecretUndecryptable
	}
	return string(plain), nil
}
