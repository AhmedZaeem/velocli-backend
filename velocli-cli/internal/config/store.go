package config

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
	"github.com/velocli/velocli/velocli-cli/internal/paths"
	"github.com/velocli/velocli/velocli-cli/internal/security"
)

type Store struct {
	v          *viper.Viper
	configFile string
	keyFile    string
}

func NewStore() (*Store, error) {
	configFile, err := paths.ConfigFile()
	if err != nil {
		return nil, err
	}
	keyFile, err := paths.KeyFile()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(configFile), 0700); err != nil {
		return nil, err
	}

	v := viper.New()
	v.SetConfigType("json")
	v.SetConfigFile(configFile)
	v.SetDefault("backend_url", "http://localhost:8080")

	_ = v.ReadInConfig()

	return &Store{
		v:          v,
		configFile: configFile,
		keyFile:    keyFile,
	}, nil
}

func (s *Store) BackendURL() string {
	return s.v.GetString("backend_url")
}

func (s *Store) SetBackendURL(url string) error {
	if url == "" {
		return errors.New("backend url required")
	}
	s.v.Set("backend_url", url)
	return s.write()
}

func (s *Store) SaveToken(token string, savedAt time.Time) error {
	key, err := security.LoadOrCreateKey(s.keyFile)
	if err != nil {
		return err
	}
	enc, err := security.Encrypt(key, []byte(token))
	if err != nil {
		return err
	}

	s.v.Set("token", enc)
	s.v.Set("token_saved_at", savedAt.UTC().Format(time.RFC3339))
	return s.write()
}

func (s *Store) ClearToken() error {
	s.v.Set("token", "")
	s.v.Set("token_saved_at", "")
	return s.write()
}

func (s *Store) write() error {
	if _, err := os.Stat(s.configFile); err == nil {
		return s.v.WriteConfig()
	}
	return s.v.WriteConfigAs(s.configFile)
}
