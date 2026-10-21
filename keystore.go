package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"lb-bar/feed"

	"github.com/egoist/mygo"
)

// The three credentials live together, as one JSON item, in the login
// keychain: one item asks for one authorization, not three. Where the
// keychain fails, they go to a file only the user can read, credentials in
// the app's data folder. The lesson of DS Search 0.2.0: the app is signed
// with a stable identifier (scripts/sign.sh), so that the keychain lets a
// newer build read what an older one saved, and a keychain that can't be
// read says why instead of looking like nothing was saved.

// secretStore is the keychain: mygo.Secrets, or a fake in tests.
type secretStore interface {
	Get(service, account string) ([]byte, error)
	Set(service, account string, v []byte) error
	Delete(service, account string) error
}

type mygoSecrets struct{}

func (mygoSecrets) Get(svc, acct string) ([]byte, error) { return mygo.Secrets.Service(svc).Get(acct) }
func (mygoSecrets) Set(svc, acct string, v []byte) error {
	return mygo.Secrets.Service(svc).Set(acct, v)
}
func (mygoSecrets) Delete(svc, acct string) error { return mygo.Secrets.Service(svc).Delete(acct) }

var errNotFound = mygo.ErrSecretNotFound

const (
	secretAccount = "longbridge-credentials"
	credFile      = "credentials"
)

type credStore struct {
	secrets secretStore
	dir     string              // the app's data folder, for the file
	env     func(string) string // os.Getenv
}

// credState is where the credentials were found, and why the keychain
// failed if it did.
type credState struct {
	creds feed.Creds
	from  string // "keychain", "file", "env" or ""
	err   string // the keychain could not be read (not "nothing saved")
}

func (s credStore) path() string { return filepath.Join(s.dir, credFile) }

func parseCreds(b []byte) (feed.Creds, bool) {
	var c feed.Creds
	if json.Unmarshal(b, &c) != nil {
		return c, false
	}
	c.AppKey, c.AppSecret, c.AccessToken = strings.TrimSpace(c.AppKey), strings.TrimSpace(c.AppSecret), strings.TrimSpace(c.AccessToken)
	return c, c.Complete()
}

// load finds the credentials: the keychain, else the file, else the
// environment (LONGPORT_* or LONGBRIDGE_*). In mode "file" the keychain is
// not asked.
func (s credStore) load(mode string) credState {
	var st credState
	if mode == "file" {
		if c, err := s.readFile(); err == nil {
			return credState{creds: c, from: "file"}
		}
	} else {
		b, err := s.secrets.Get("", secretAccount)
		switch {
		case err == nil:
			if c, ok := parseCreds(b); ok {
				return credState{creds: c, from: "keychain"}
			}
		case errors.Is(err, errNotFound):
		default:
			st.err = keychainErr(err)
		}
		if c, err := s.readFile(); err == nil {
			st.creds, st.from = c, "file"
			return st
		}
	}
	if c := s.fromEnv(); c.Complete() {
		st.creds, st.from = c, "env"
	}
	return st
}

func (s credStore) fromEnv() feed.Creds {
	get := func(names ...string) string {
		for _, n := range names {
			if v := strings.TrimSpace(s.env(n)); v != "" {
				return v
			}
		}
		return ""
	}
	return feed.Creds{
		AppKey:      get("LONGBRIDGE_APP_KEY", "LONGPORT_APP_KEY"),
		AppSecret:   get("LONGBRIDGE_APP_SECRET", "LONGPORT_APP_SECRET"),
		AccessToken: get("LONGBRIDGE_ACCESS_TOKEN", "LONGPORT_ACCESS_TOKEN"),
	}
}

func (s credStore) readFile() (feed.Creds, error) {
	b, err := os.ReadFile(s.path())
	if err != nil {
		return feed.Creds{}, err
	}
	c, ok := parseCreds(b)
	if !ok {
		return c, errors.New("credentials: incomplete")
	}
	return c, nil
}

func (s credStore) writeFile(c feed.Creds) error {
	b, _ := json.Marshal(c)
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	tmp := s.path() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path())
}

// save keeps c where mode says ("file", or "" for the keychain), and says
// where it went. A keychain that fails to keep them, or to give them
// back, falls back to the file; warn says why.
func (s credStore) save(c feed.Creds, mode string) (from, warn string, err error) {
	b, _ := json.Marshal(c)
	if mode != "file" {
		err := s.secrets.Set("", secretAccount, b)
		if err == nil {
			var got []byte
			if got, err = s.secrets.Get("", secretAccount); err == nil {
				if back, _ := parseCreds(got); back != c {
					err = errors.New("读回的内容不一致")
				}
			}
		}
		if err == nil {
			_ = os.Remove(s.path())
			return "keychain", "", nil
		}
		warn = keychainErr(err)
	}
	if err := s.writeFile(c); err != nil {
		if warn != "" {
			return "", "", fmt.Errorf("钥匙串：%s；本地文件：%v", warn, err)
		}
		return "", "", fmt.Errorf("写入本地文件失败：%v", err)
	}
	if mode == "file" {
		_ = s.secrets.Delete("", secretAccount)
	}
	return "file", warn, nil
}

// remove deletes the credentials everywhere the app keeps them.
func (s credStore) remove() error {
	err := s.secrets.Delete("", secretAccount)
	if errors.Is(err, errNotFound) {
		err = nil
	}
	if e := os.Remove(s.path()); e != nil && !errors.Is(e, os.ErrNotExist) && err == nil {
		err = e
	}
	if err != nil {
		return errors.New(keychainErr(err))
	}
	return nil
}

var osStatus = regexp.MustCompile(`OSStatus (-?\d+)`)

// keychainErr says in Chinese why the keychain failed.
func keychainErr(err error) string {
	why := err.Error()
	if m := osStatus.FindStringSubmatch(why); m != nil {
		switch m[1] {
		case "-128":
			why = "授权请求被取消"
		case "-25293":
			why = "授权失败"
		case "-25308":
			why = "此时不允许弹出授权"
		case "-25291", "-25294", "-25295":
			why = "找不到可用的钥匙串"
		case "-34018":
			why = "App 缺少钥匙串权限"
		default:
			why = "错误 " + m[1]
		}
	}
	return "钥匙串不可用（" + why + "）"
}

// mask shows a credential as ••••1234.
func mask(s string) string {
	r := []rune(s)
	if len(r) <= 8 {
		return "••••"
	}
	return "••••" + string(r[len(r)-4:])
}

// tokenExpiry reads when an access token expires: its JWT exp claim.
// The legacy access tokens of the developer center are JWTs.
func tokenExpiry(token string) (time.Time, bool) {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(token), "Bearer "), ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp json.Number `json:"exp"`
	}
	if json.Unmarshal(b, &claims) != nil {
		return time.Time{}, false
	}
	n, err := claims.Exp.Int64()
	if err != nil || n <= 0 {
		return time.Time{}, false
	}
	return time.Unix(n, 0), true
}

func storeName(from string) string {
	switch from {
	case "keychain":
		return "macOS「登录」钥匙串"
	case "file":
		return "本地文件（备用）"
	case "env":
		return "环境变量 LONGPORT_*"
	}
	return "未保存"
}
