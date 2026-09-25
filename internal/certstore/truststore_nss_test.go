//go:build !windows

package certstore

import (
	"os"
	"path/filepath"
	"testing"
)

// The tests swap firefoxProfiles, so they cannot run in parallel.

func TestInstallNSSSucceedsIfAnyDatabaseTakesCA(t *testing.T) {
	_, snapDB := setUpFakeNSS(t)
	lockNSSDB(t, snapDB)

	if err := (&DiskCertStore{}).installNSS(true); err != nil {
		t.Fatalf("got error %v, want nil", err)
	}
}

func TestInstallNSSFailsIfNoDatabaseTakesCA(t *testing.T) {
	userDB, snapDB := setUpFakeNSS(t)
	lockNSSDB(t, userDB)
	lockNSSDB(t, snapDB)

	if err := (&DiskCertStore{}).installNSS(true); err == nil {
		t.Fatal("got nil error, want an error")
	}
}

// fakeCertutil adds the CA to a database by creating a zen-ca file in it, and
// fails to if the database contains a locked file.
const fakeCertutil = `#!/bin/sh
op=$1
while [ $# -gt 0 ]; do
	[ "$1" = -d ] && dir=${2#*:}
	shift
done
case $op in
-A) [ ! -e "$dir/locked" ] && touch "$dir/zen-ca" ;;
-V) [ -e "$dir/zen-ca" ] ;;
esac
`

// setUpFakeNSS points HOME at a temporary directory holding two NSS databases,
// the user one and the Chromium snap one, hides Firefox profiles, and puts
// fakeCertutil first on PATH.
func setUpFakeNSS(t *testing.T) (userDB, snapDB string) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	userDB = filepath.Join(home, ".pki/nssdb")
	snapDB = filepath.Join(home, "snap/chromium/current/.pki/nssdb")
	for _, db := range []string{userDB, snapDB} {
		if err := os.MkdirAll(db, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(db, "cert9.db"), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}

	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "certutil"), []byte(fakeCertutil), 0700); err != nil { // #nosec G306 -- the fake certutil must be executable.
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	saved := firefoxProfiles
	firefoxProfiles = nil
	t.Cleanup(func() { firefoxProfiles = saved })

	return userDB, snapDB
}

func lockNSSDB(t *testing.T, db string) {
	if err := os.WriteFile(filepath.Join(db, "locked"), nil, 0600); err != nil {
		t.Fatal(err)
	}
}
