package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"

	"argodeck/internal/argocd"
)

// Auth methods supported, mirroring `argocd login` options.
const (
	AuthSSO      = "sso"      // OIDC via Dex or external provider (argocd login --sso)
	AuthPassword = "password" // local account (argocd login --username/--password)
	AuthToken    = "token"    // API/project token (argocd --auth-token)
	AuthCLI      = "cli"      // credentials imported from ~/.config/argocd/config
)

// Context is one Argo CD API server the user manages.
type Context struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Server         string            `json:"server"`
	AuthType       string            `json:"authType"`
	Insecure       bool              `json:"insecure"`
	CAFile         string            `json:"caFile,omitempty"`
	ClientCertFile string            `json:"clientCertFile,omitempty"`
	ClientKeyFile  string            `json:"clientKeyFile,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	SSOPort        int               `json:"ssoPort,omitempty"`
	SSONoOffline   bool              `json:"ssoNoOffline,omitempty"`
	Username       string            `json:"username,omitempty"`
	Color          string            `json:"color,omitempty"`
	Disabled       bool              `json:"disabled,omitempty"`
}

func (c Context) ClientOptions() argocd.Options {
	return argocd.Options{
		Server: c.Server, Insecure: c.Insecure, CAFile: c.CAFile,
		ClientCertFile: c.ClientCertFile, ClientKeyFile: c.ClientKeyFile, Headers: c.Headers,
	}
}

type Prefs struct {
	Theme string `json:"theme,omitempty"`
}

type file struct {
	Contexts []Context `json:"contexts"`
	Prefs    Prefs     `json:"prefs"`
}

type Store struct {
	mu   sync.Mutex
	dir  string
	data file
	// fallback secrets when the OS keychain is unavailable (e.g. headless Linux)
	fallback  map[string]argocd.Credentials
	noKeyring bool
}

const keyringService = "argodeck"

// Open loads the config from the user config dir. ARGODECK_CONFIG_DIR overrides
// the location and ARGODECK_NO_KEYRING=1 keeps secrets in a 0600 file instead of
// the OS keychain (useful for tests and headless setups).
func Open() (*Store, error) {
	dir := os.Getenv("ARGODECK_CONFIG_DIR")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(base, "argodeck")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, fallback: map[string]argocd.Credentials{}, noKeyring: os.Getenv("ARGODECK_NO_KEYRING") == "1"}
	if b, err := os.ReadFile(filepath.Join(dir, "config.json")); err == nil {
		_ = json.Unmarshal(b, &s.data)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "secrets.json")); err == nil {
		_ = json.Unmarshal(b, &s.fallback)
	}
	return s, nil
}

func (s *Store) saveLocked() error {
	b, _ := json.MarshalIndent(s.data, "", "  ")
	return os.WriteFile(filepath.Join(s.dir, "config.json"), b, 0o600)
}

func (s *Store) Contexts() []Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Context(nil), s.data.Contexts...)
}

func (s *Store) Get(id string) (Context, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.data.Contexts {
		if c.ID == id {
			return c, true
		}
	}
	return Context{}, false
}

func NewID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Upsert stores a context, assigning an ID if new.
func (s *Store) Upsert(c Context) (Context, error) {
	c.Server = strings.TrimRight(strings.TrimSpace(c.Server), "/")
	if c.Server == "" {
		return c, errors.New("server URL is required")
	}
	if !strings.Contains(c.Server, "://") {
		c.Server = "https://" + c.Server
	}
	if c.Name == "" {
		c.Name = strings.TrimPrefix(strings.TrimPrefix(c.Server, "https://"), "http://")
	}
	if c.AuthType == "" {
		c.AuthType = AuthSSO
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.ID == "" {
		c.ID = NewID()
		s.data.Contexts = append(s.data.Contexts, c)
	} else {
		found := false
		for i := range s.data.Contexts {
			if s.data.Contexts[i].ID == c.ID {
				s.data.Contexts[i] = c
				found = true
			}
		}
		if !found {
			s.data.Contexts = append(s.data.Contexts, c)
		}
	}
	return c, s.saveLocked()
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	out := s.data.Contexts[:0]
	for _, c := range s.data.Contexts {
		if c.ID != id {
			out = append(out, c)
		}
	}
	s.data.Contexts = out
	err := s.saveLocked()
	s.mu.Unlock()
	s.SetCredentials(id, argocd.Credentials{})
	return err
}

func (s *Store) Prefs() Prefs {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Prefs
}

func (s *Store) SetPrefs(p Prefs) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Prefs = p
	return s.saveLocked()
}

// ---- secrets ---------------------------------------------------------------

func (s *Store) Credentials(id string) argocd.Credentials {
	var cr argocd.Credentials
	if s.noKeyring {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.fallback[id]
	}
	if v, err := keyring.Get(keyringService, id); err == nil {
		if json.Unmarshal([]byte(v), &cr) == nil {
			return cr
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fallback[id]
}

func (s *Store) SetCredentials(id string, cr argocd.Credentials) {
	empty := cr == (argocd.Credentials{})
	if s.noKeyring {
		s.mu.Lock()
		defer s.mu.Unlock()
		if empty {
			delete(s.fallback, id)
		} else {
			s.fallback[id] = cr
		}
		s.writeFallbackLocked()
		return
	}
	if empty {
		_ = keyring.Delete(keyringService, id)
	} else {
		b, _ := json.Marshal(cr)
		if err := keyring.Set(keyringService, id, string(b)); err == nil {
			s.mu.Lock()
			if _, had := s.fallback[id]; had {
				delete(s.fallback, id)
				s.writeFallbackLocked()
			}
			s.mu.Unlock()
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if empty {
		delete(s.fallback, id)
	} else {
		s.fallback[id] = cr
	}
	s.writeFallbackLocked()
}

func (s *Store) writeFallbackLocked() {
	b, _ := json.Marshal(s.fallback)
	_ = os.WriteFile(filepath.Join(s.dir, "secrets.json"), b, 0o600)
}

// ---- argocd CLI config import ---------------------------------------------

type cliConfig struct {
	Contexts []struct {
		Name   string `yaml:"name"`
		Server string `yaml:"server"`
		User   string `yaml:"user"`
	} `yaml:"contexts"`
	Servers []struct {
		Server            string `yaml:"server"`
		Insecure          bool   `yaml:"insecure"`
		PlainText         bool   `yaml:"plain-text"`
		GRPCWebRootPath   string `yaml:"grpc-web-root-path"`
		Core              bool   `yaml:"core"`
		ClientCertFile    string `yaml:"client-cert-file"`
		ClientCertKeyFile string `yaml:"client-cert-key-file"`
	} `yaml:"servers"`
	Users []struct {
		Name         string `yaml:"name"`
		AuthToken    string `yaml:"auth-token"`
		RefreshToken string `yaml:"refresh-token"`
	} `yaml:"users"`
}

func CLIConfigPath() string {
	if d := os.Getenv("ARGOCD_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "config")
	}
	home, _ := os.UserHomeDir()
	p := filepath.Join(home, ".config", "argocd", "config")
	if _, err := os.Stat(p); err != nil {
		if _, err2 := os.Stat(filepath.Join(home, ".argocd", "config")); err2 == nil {
			return filepath.Join(home, ".argocd", "config")
		}
	}
	return p
}

// ImportCLI reads the argocd CLI config and adds (or refreshes) one context per
// CLI context, carrying over its tokens. Core-mode contexts are skipped.
func (s *Store) ImportCLI() ([]Context, error) {
	b, err := os.ReadFile(CLIConfigPath())
	if err != nil {
		return nil, err
	}
	var cc cliConfig
	if err := yaml.Unmarshal(b, &cc); err != nil {
		return nil, err
	}
	existing := map[string]Context{}
	for _, c := range s.Contexts() {
		existing[c.Server] = c
	}
	var out []Context
	for _, x := range cc.Contexts {
		ctx := Context{Name: x.Name, AuthType: AuthCLI}
		skip := false
		for _, sv := range cc.Servers {
			if sv.Server != x.Server {
				continue
			}
			if sv.Core {
				skip = true
			}
			scheme := "https://"
			if sv.PlainText {
				scheme = "http://"
			}
			ctx.Server = scheme + strings.TrimRight(sv.Server, "/")
			if sv.GRPCWebRootPath != "" {
				ctx.Server += "/" + strings.Trim(sv.GRPCWebRootPath, "/")
			}
			ctx.Insecure = sv.Insecure
			ctx.ClientCertFile = sv.ClientCertFile
			ctx.ClientKeyFile = sv.ClientCertKeyFile
		}
		if skip || ctx.Server == "" {
			continue
		}
		var cr argocd.Credentials
		for _, u := range cc.Users {
			if u.Name == x.User {
				cr = argocd.Credentials{Token: u.AuthToken, RefreshToken: u.RefreshToken}
			}
		}
		if prev, ok := existing[ctx.Server]; ok {
			ctx.ID = prev.ID
			ctx.Color = prev.Color
			if prev.AuthType != AuthCLI {
				ctx.AuthType = prev.AuthType
			}
		}
		saved, err := s.Upsert(ctx)
		if err != nil {
			return out, err
		}
		if cr.Token != "" {
			s.SetCredentials(saved.ID, cr)
		}
		out = append(out, saved)
	}
	return out, nil
}
