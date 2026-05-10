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
	"os"
	"os/exec"
)

var firefoxProfiles = []string{os.Getenv("HOME") + "/.mozilla/firefox/*",
	os.Getenv("HOME") + "/snap/firefox/common/.mozilla/firefox/*"}

const (
	// caFolderName defines the name of the folder where the root CA certificate and key are stored.
	caFolderName = "certs"
	// systemTrustFilename is the name of the file that contains the system trust settings.
	systemTrustFilename = "zen-rootCA"
)

// getSystemTrustInfo identifies the system's trust store
// and returns the filename the root CA should be stored in and the command to update the trust store.
func getSystemTrustInfo() (certFilename string, command []string, err error) {
	var certPath string
	switch {
	case pathExists("/etc/pki/ca-trust/source/anchors/"):
		certPath = "/etc/pki/ca-trust/source/anchors/%s.pem"
		command = []string{"update-ca-trust", "extract"}
	case pathExists("/usr/local/share/ca-certificates/"):
		certPath = "/usr/local/share/ca-certificates/%s.crt"
		command = []string{"update-ca-certificates"}
	case pathExists("/etc/ca-certificates/trust-source/anchors/"):
		certPath = "/etc/ca-certificates/trust-source/anchors/%s.crt"
		command = []string{"trust", "extract-compat"}
	case pathExists("/usr/share/pki/trust/anchors"):
		certPath = "/usr/share/pki/trust/anchors/%s.pem"
		command = []string{"update-ca-certificates"}
	default:
		return "", []string{}, errors.New("system trust store not found")
	}

	certFilename = fmt.Sprintf(certPath, systemTrustFilename)

	return certFilename, command, nil
}

// installCATrust installs the CA into the system trust store.
func (cs *DiskCertStore) installCATrust() error {
	certFilename, systemTrustCommand, err := getSystemTrustInfo()
	if err != nil {
		return fmt.Errorf("failed to get system trust store: %w", err)
	}

	cmd := exec.Command("pkexec", "tee", certFilename)
	cmd.Stdin = bytes.NewReader(cs.certData)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("write cert to %s: %w\n%s", certFilename, err, out)
	}

	cmd = exec.Command("pkexec", systemTrustCommand...)
	out, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("system trust command: %w\n%s", err, out)
	}

	return nil
}

// uninstallCATrust uninstalls the CA from the system trust store.
func (cs *DiskCertStore) uninstallCATrust() error {
	certFilename, systemTrustCommand, err := getSystemTrustInfo()
	if err != nil {
		return fmt.Errorf("failed to get system trust store: %w", err)
	}

	cmd := exec.Command("pkexec", "rm", "-f", certFilename)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("remove cert from %s: %w\n%s", certFilename, err, out)
	}

	cmd = exec.Command("pkexec", systemTrustCommand...)
	out, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("system trust command: %w\n%s", err, out)
	}

	return nil
}
