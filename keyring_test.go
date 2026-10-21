package main

import (
	"errors"
	"os"
	"testing"
)

// TestSystemKeyring round-trips a made-up secret through the system's
// store (mygo.Secrets): the Keychain, the Credential Manager, or a Secret
// Service on Linux. Only with LBBAR_KEYRING=1, as it writes to the user's
// store; it deletes what it wrote.
func TestSystemKeyring(t *testing.T) {
	if os.Getenv("LBBAR_KEYRING") != "1" {
		t.Skip("set LBBAR_KEYRING=1 to use the system's credential store")
	}
	s := mygoSecrets{}
	const svc, acct = "dev.horizon.lbbar.test", "roundtrip"
	want := []byte(`{"appKey":"example","appSecret":"example","accessToken":"example"}`)
	if err := s.Set(svc, acct, want); err != nil {
		if noStore(err) {
			t.Skipf("no credential store here: %s", keychainErr(err))
		}
		t.Fatal(err)
	}
	got, err := s.Get(svc, acct)
	if err != nil || string(got) != string(want) {
		t.Fatalf("get: %v %q", err, got)
	}
	if err := s.Delete(svc, acct); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(svc, acct); !errors.Is(err, errNotFound) {
		t.Errorf("after delete: %v", err)
	}
	// And the credentials' store over it, with the file fallback unused.
	ks := credStore{secrets: s, dir: t.TempDir(), env: func(string) string { return "" }}
	from, warn, err := ks.save(testCreds, "")
	if err != nil || from != "keychain" || warn != "" {
		t.Fatalf("save: %q %q %v", from, warn, err)
	}
	defer ks.remove()
	if st := ks.load(""); st.creds != testCreds || st.from != "keychain" {
		t.Errorf("load: %+v", st.from)
	}
}
