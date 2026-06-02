// apikey.go — хэширование API-ключей для хранения hash+prefix (без открытого текста).
package store

import (
	"crypto/sha256"
	"encoding/hex"
)

// hashAPIKey возвращает SHA-256 (hex) ключа — то, что хранится и по чему ищется.
func hashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// keyPrefix возвращает первые 16 символов ключа (для идентификации в UI, не секрет).
func keyPrefix(key string) string {
	if len(key) <= 16 {
		return key
	}
	return key[:16]
}
