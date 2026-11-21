package store

import (
	"errors"
	"slices"

	"github.com/zalando/go-keyring"

	"github.com/lichenghsu/synccraft/internal/conn"
)

// Secrets stores secret strings by key. Get returns "" for a missing key and
// Delete of a missing key is not an error.
type Secrets interface {
	Set(key, value string) error
	Get(key string) (string, error)
	Delete(key string) error
}

// Keyring stores secrets in the OS keychain (Keychain, Credential Manager, Secret Service).
type Keyring struct {
	Service string
}

func (k Keyring) Set(key, value string) error { return keyring.Set(k.Service, key, value) }

func (k Keyring) Get(key string) (string, error) {
	v, err := keyring.Get(k.Service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return v, err
}

func (k Keyring) Delete(key string) error {
	if err := keyring.Delete(k.Service, key); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

// Secret names: keychain key suffixes and the values accepted in update lists.
const (
	SecretPassword      = "password"
	SecretSSHPassword   = "ssh-password"
	SecretSSHPassphrase = "ssh-passphrase"
)

var secretNames = []string{SecretPassword, SecretSSHPassword, SecretSSHPassphrase}

// AllSecrets lists every secret name, for "update everything".
func AllSecrets() []string { return slices.Clone(secretNames) }

type secretField struct {
	name string
	ptr  *string
}

func secretKey(id, name string) string { return id + "/" + name }

// secretFields returns the secret fields present in cfg (SSH ones only with a tunnel).
func secretFields(cfg *conn.ConnectionConfig) []secretField {
	fs := []secretField{{SecretPassword, &cfg.Password}}
	if cfg.SSH != nil {
		fs = append(fs, secretField{SecretSSHPassword, &cfg.SSH.Password}, secretField{SecretSSHPassphrase, &cfg.SSH.Passphrase})
	}
	return fs
}

// allSecretFields always covers every name, so removing the tunnel deletes its secrets.
func allSecretFields(cfg *conn.ConnectionConfig) []secretField {
	if cfg.SSH == nil {
		var empty1, empty2 string
		return []secretField{{SecretPassword, &cfg.Password}, {SecretSSHPassword, &empty1}, {SecretSSHPassphrase, &empty2}}
	}
	return secretFields(cfg)
}

// copyConfig deep-copies the pointer fields so callers' configs are never mutated.
func copyConfig(cfg conn.ConnectionConfig) conn.ConnectionConfig {
	if cfg.SSH != nil {
		ssh := *cfg.SSH
		cfg.SSH = &ssh
	}
	if cfg.TLS != nil {
		tls := *cfg.TLS
		cfg.TLS = &tls
	}
	return cfg
}

func withoutSecrets(cfg conn.ConnectionConfig) conn.ConnectionConfig {
	cfg = copyConfig(cfg)
	for _, f := range secretFields(&cfg) {
		*f.ptr = ""
	}
	return cfg
}
