package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"lb-bar/alerts"
	"lb-bar/feed"

	"github.com/egoist/mygo/ui"
)

// memSecrets is a keychain in memory.
type memSecrets struct {
	m    map[string]string
	fail error
	sets int
}

func (s *memSecrets) Get(svc, acct string) ([]byte, error) {
	if s.fail != nil {
		return nil, s.fail
	}
	v, ok := s.m[svc+"|"+acct]
	if !ok {
		return nil, errNotFound
	}
	return []byte(v), nil
}

func (s *memSecrets) Set(svc, acct string, v []byte) error {
	if s.fail != nil {
		return s.fail
	}
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.sets++
	s.m[svc+"|"+acct] = string(v)
	return nil
}

func (s *memSecrets) Delete(svc, acct string) error {
	if _, ok := s.m[svc+"|"+acct]; !ok {
		return errNotFound
	}
	delete(s.m, svc+"|"+acct)
	return nil
}

var errDenied = errors.New("keychain: user canceled (-128)")

// testNow is a Tuesday in Asia/Shanghai, 14:05: HK and CN trading, the
// US closed.
var testNow = time.Date(2026, 10, 20, 14, 5, 0, 0, feed.Shanghai)

// testExpiry is when the example token expires.
var testExpiry = time.Date(2027, 1, 19, 10, 16, 0, 0, feed.Shanghai)

// fakeJWT is a token of the shape the developer center hands out, made
// up, expiring at exp.
func fakeJWT(exp time.Time) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + enc([]byte(fmt.Sprintf(`{"exp":%d}`, exp.Unix()))) + ".example-signature-0000"
}

var testCreds = feed.Creds{AppKey: "example-app-key-a1b2", AppSecret: "example-app-secret-c3d4", AccessToken: fakeJWT(testExpiry)}

// harness is an app on the mock provider, all on the test's goroutine:
// what the shell would run later waits in its queue.
type harness struct {
	*app
	mock     *feed.Mock
	mu       sync.Mutex
	queue    []func()
	opened   []string
	notified []string
	resized  []bool
	hidden   int
	quits    int
	copied   string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	m := &feed.Mock{Now: func() time.Time { return testNow }}
	h := &harness{mock: m}
	a := newApp(defaultConfig(), testCreds, "keychain", func(feed.Creds, feed.Options) (feed.Provider, error) { return m, nil })
	h.app = a
	dir := t.TempDir()
	a.dataDir = dir
	a.keys = credStore{secrets: &memSecrets{}, dir: dir, env: func(string) string { return "" }}
	a.alerts = alerts.Open(filepath.Join(dir, "alerts.json"))
	a.aiKeys = aiKeyStore{secrets: &memSecrets{}, dir: dir, env: func(string) string { return "" }}
	a.now = func() time.Time { return testNow }
	a.spawn = func(fn func()) { fn() }
	a.later = func(_ time.Duration, fn func()) {
		h.mu.Lock()
		h.queue = append(h.queue, fn)
		h.mu.Unlock()
	}
	a.open = func(u string) { h.opened = append(h.opened, u) }
	a.notify = func(id, title, body string) { h.notified = append(h.notified, title+" | "+body) }
	a.resize = func(on bool) { h.resized = append(h.resized, on) }
	a.hide = func() { h.hidden++ }
	a.quit = func() { h.quits++ }
	a.copy = func(s string) { h.copied = s }
	a.connect()
	h.run()
	return h
}

// run runs what waits, and what that queues.
func (h *harness) run() {
	for {
		h.mu.Lock()
		q := h.queue
		h.queue = nil
		h.mu.Unlock()
		if len(q) == 0 {
			return
		}
		for _, fn := range q {
			fn()
		}
	}
}

func (h *harness) tester() *ui.Tester {
	tt := ui.NewTester(h.view, panelWidth, panelHeight)
	tt.Frame()
	return tt
}

// key presses a key and runs what it started.
func (h *harness) key(tt *ui.Tester, mods ui.Modifiers, k ui.Key) {
	tt.Key(mods, k)
	h.run()
	tt.Frame()
}

func (h *harness) typ(tt *ui.Tester, s string) {
	tt.Type(s)
	h.run()
	tt.Frame()
}

func save(t *testing.T, path string, tt *ui.Tester) {
	t.Helper()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
