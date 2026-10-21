package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// The DeepSeek API key lives in the login keychain, an item of its own
// (the Longbridge credentials are another), or, where the keychain fails,
// in deepseek-key (0600) in the app's data folder; else DEEPSEEK_API_KEY.

const (
	aiSecretAccount = "deepseek-api-key"
	aiKeyFile       = "deepseek-key"
)

type aiKeyStore struct {
	secrets secretStore
	dir     string
	env     func(string) string
}

// aiKeyState is where the key was found, and why the keychain failed.
type aiKeyState struct {
	key, from, err string
}

func (s aiKeyStore) path() string { return filepath.Join(s.dir, aiKeyFile) }

func (s aiKeyStore) load(mode string) aiKeyState {
	var st aiKeyState
	if mode == "file" {
		if k, err := s.readFile(); err == nil && k != "" {
			return aiKeyState{key: k, from: "file"}
		}
	} else {
		b, err := s.secrets.Get("", aiSecretAccount)
		switch {
		case err == nil && strings.TrimSpace(string(b)) != "":
			return aiKeyState{key: strings.TrimSpace(string(b)), from: "keychain"}
		case err == nil || errors.Is(err, errNotFound):
		default:
			st.err = keychainErr(err)
		}
		if k, err := s.readFile(); err == nil && k != "" {
			st.key, st.from = k, "file"
			return st
		}
	}
	if k := strings.TrimSpace(s.env("DEEPSEEK_API_KEY")); k != "" {
		st.key, st.from = k, "env"
	}
	return st
}

func (s aiKeyStore) readFile() (string, error) {
	b, err := os.ReadFile(s.path())
	return strings.TrimSpace(string(b)), err
}

func (s aiKeyStore) writeFile(key string) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	tmp := s.path() + ".tmp"
	if err := os.WriteFile(tmp, []byte(key+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path())
}

// save keeps the key in the keychain, read back to be sure, or in the
// file: there with mode "file", or when the keychain fails (warn says
// why).
func (s aiKeyStore) save(key, mode string) (from, warn string, err error) {
	if mode != "file" {
		err := s.secrets.Set("", aiSecretAccount, []byte(key))
		if err == nil {
			if b, e := s.secrets.Get("", aiSecretAccount); e != nil || strings.TrimSpace(string(b)) != key {
				err = errors.New("写入后读不回来")
			}
		}
		if err == nil {
			_ = os.Remove(s.path())
			return "keychain", "", nil
		}
		warn = "钥匙串保存失败（" + keychainErr(err) + "）"
	}
	if err := s.writeFile(key); err != nil {
		return "", warn, err
	}
	if mode != "file" {
		return "file", warn, nil
	}
	_ = s.secrets.Delete("", aiSecretAccount)
	return "file", "", nil
}

func (s aiKeyStore) remove() error {
	err := s.secrets.Delete("", aiSecretAccount)
	if errors.Is(err, errNotFound) {
		err = nil
	}
	if e := os.Remove(s.path()); e != nil && !errors.Is(e, os.ErrNotExist) && err == nil {
		err = e
	}
	return err
}

func aiStoreName(from string) string {
	if from == "env" {
		return "环境变量 DEEPSEEK_API_KEY"
	}
	return storeName(from)
}
