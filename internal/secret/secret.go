package secret

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SecretsStore holds project-specific random secrets.
type SecretsStore struct {
	Secrets map[string]string `json:"secrets"`
}

// GenerateRandomSecret generates a cryptographically secure random hex string of the specified byte length.
func GenerateRandomSecret(byteLength int) (string, error) {
	if byteLength <= 0 {
		byteLength = 16 // 32 hex chars
	}
	bytes := make([]byte, byteLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure random bytes: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// SecretsFilePath returns the standard path to .sld/secrets.json.
func SecretsFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".sld", "secrets.json")
}

// LoadSecrets reads the project secrets file. Returns an empty store if the file does not exist.
func LoadSecrets(projectRoot string) (*SecretsStore, error) {
	filePath := SecretsFilePath(projectRoot)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &SecretsStore{Secrets: make(map[string]string)}, nil
		}
		return nil, fmt.Errorf("failed to read secrets file: %w", err)
	}

	var store SecretsStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("failed to parse secrets file: %w", err)
	}
	if store.Secrets == nil {
		store.Secrets = make(map[string]string)
	}

	return &store, nil
}

// SaveSecrets writes the secrets store to .sld/secrets.json with strict 0600 permissions.
func SaveSecrets(projectRoot string, store *SecretsStore) error {
	dirPath := filepath.Join(projectRoot, ".sld")
	if err := os.MkdirAll(dirPath, 0700); err != nil {
		return fmt.Errorf("failed to create .sld directory: %w", err)
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode secrets: %w", err)
	}

	filePath := SecretsFilePath(projectRoot)
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write secrets file: %w", err)
	}

	return nil
}

// GetOrGenerate retrieves an existing secret or creates, saves, and returns a new random secret.
func GetOrGenerate(projectRoot string, key string, byteLength int) (string, error) {
	store, err := LoadSecrets(projectRoot)
	if err != nil {
		return "", err
	}

	if val, exists := store.Secrets[key]; exists && val != "" {
		return val, nil
	}

	newSecret, err := GenerateRandomSecret(byteLength)
	if err != nil {
		return "", err
	}

	store.Secrets[key] = newSecret
	if err := SaveSecrets(projectRoot, store); err != nil {
		return "", err
	}

	return newSecret, nil
}
