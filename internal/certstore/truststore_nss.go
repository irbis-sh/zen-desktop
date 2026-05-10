package certstore

/*
This file contains code from the mkcert project,
licensed under the BSD 3-Clause License:
Copyright (c) 2018 The mkcert Authors. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:
- Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
- Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
- Neither the name of Google Inc. nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

func getNSSInfo() (hasNSS bool, hasCertutil bool, certutilPath string) {
	allPaths := append(append([]string{}, getNssDbs()...), getFirefoxPaths()...)
	hasNSS = slices.ContainsFunc(allPaths, pathExists)

	switch runtime.GOOS {
	case "darwin":
		switch {
		case binaryExists("certutil"):
			certutilPath, _ = exec.LookPath("certutil")
			hasCertutil = true
		case binaryExists("/usr/local/opt/nss/bin/certutil"):
			certutilPath, _ = exec.LookPath("/usr/local/opt/nss/bin/certutil")
			hasCertutil = true
		default:
			out, err := exec.Command("brew", "--prefix", "nss").Output()
			if err == nil {
				certutilPath = filepath.Join(strings.TrimSpace(string(out)), "bin", "certutil")
				hasCertutil = pathExists(certutilPath)
			}
		}
	case "linux":
		if hasCertutil = binaryExists("certutil"); hasCertutil {
			certutilPath, _ = exec.LookPath("certutil")
		}
	}

	return
}

func (cs *DiskCertStore) checkNSS() bool {
	_, hasCertutil, certutilpath := getNSSInfo()

	if !hasCertutil {
		return false
	}

	success := true
	profileCount := cs.forEachNSSProfile(func(profile string) {
		err := exec.Command(certutilpath, "-V", "-d", profile, "-u", "L", "-n", certCommonName).Run()
		if err != nil {
			success = false
		}
	})

	return profileCount > 0 && success
}

func (cs *DiskCertStore) installNSS() error {
	hasNSS, hasCertutil, certutilPath := getNSSInfo()
	if !hasNSS {
		return fmt.Errorf("no NSS browsers found")
	}

	if !hasCertutil {
		return errors.New("no certutil found")
	}

	if cs.forEachNSSProfile(func(profile string) {
		cmd := exec.Command(certutilPath, "-A", "-d", profile, "-t", "C,,", "-n", certCommonName, "-i", cs.certPath)
		out, err := execCertutil(cmd)
		if err != nil {
			log.Printf("failed to install cert in %s: %v (%q)", profile, err, out)
		}
	}) == 0 {
		return errors.New("no security databases found")
	}

	if !cs.checkNSS() {
		return errors.New("failed to install NSS, profiles have not been created")
	}
	return nil
}

func (cs *DiskCertStore) uninstallNSS() error {
	_, hasCertutil, certutilPath := getNSSInfo()

	if !hasCertutil {
		return errors.New("no certutil found")
	}

	if cs.forEachNSSProfile(func(profile string) {
		err := exec.Command(certutilPath, "-V", "-d", profile, "-u", "L", "-n", certCommonName).Run()
		if err != nil {
			return
		}
		cmd := exec.Command(certutilPath, "-D", "-d", profile, "-n", certCommonName)
		out, err := execCertutil(cmd)
		if err != nil {
			log.Printf("failed to uninstall cert from %s: %v (%q)", profile, err, out)
		}
	}) == 0 {
		return errors.New("no security databases found")
	}
	return nil
}

func (cs *DiskCertStore) forEachNSSProfile(f func(profile string)) (found int) {
	profiles := getNssDbs()
	for _, ff := range firefoxProfiles {
		matches, err := filepath.Glob(ff)
		if err == nil {
			profiles = append(profiles, matches...)
		}
	}

	for _, profile := range profiles {
		if stat, err := os.Stat(profile); err != nil || !stat.IsDir() {
			continue
		}
		if pathExists(filepath.Join(profile, "cert9.db")) {
			f("sql:" + profile)
			found++
		}
		if pathExists(filepath.Join(profile, "cert8.db")) {
			f("dbm:" + profile)
			found++
		}
	}
	return
}

// getFirefoxPaths lists all possible firefox installations.
func getFirefoxPaths() []string {
	firefoxPaths := []string{
		"/snap/firefox",
		"C:\\Program Files\\Mozilla Firefox",
	}

	firefoxPatterns := []string{
		"/usr/bin/firefox*",
		"/Applications/Firefox*",
	}

	for _, pattern := range firefoxPatterns {
		matches, err := filepath.Glob(pattern)
		if err == nil {
			firefoxPaths = append(firefoxPaths, matches...)
		}
	}

	return firefoxPaths
}

func getNssDbs() []string {
	return []string{
		filepath.Join(os.Getenv("HOME"), ".pki/nssdb"),
		filepath.Join(os.Getenv("HOME"), "snap/chromium/current/.pki/nssdb"),
		"/etc/pki/nssdb",
	}
}

// execCertutil will execute a "certutil" command and if needed re-execute
// the command with "pkexec" to work around file permissions.
func execCertutil(cmd *exec.Cmd) ([]byte, error) {
	out, err := cmd.CombinedOutput()
	if err != nil && bytes.Contains(out, []byte("SEC_ERROR_READ_ONLY")) && runtime.GOOS == "linux" {
		origArgs := cmd.Args[1:]
		cmd = exec.Command("pkexec", cmd.Path) // #nosec G204
		cmd.Args = append(cmd.Args, origArgs...)
		out, err = cmd.CombinedOutput()
	}
	return out, err
}

func binaryExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
