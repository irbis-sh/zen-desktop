package sysproxy

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/hashicorp/go-multierror"
	"howett.net/plist"
)

var (
	//go:embed exclusions/darwin.txt
	platformSpecificExcludedHosts []byte

	// networkServices remembers the services we modified, for unsetSystemProxy.
	networkServices []string
)

// setSystemProxy sets the system proxy PAC URL.
func setSystemProxy(pacURL string) error {
	svcs, err := discoverNetworkServices()
	if err != nil {
		return fmt.Errorf("discover network services: %v", err)
	}
	networkServices = svcs

	cmds := make([][]string, 0, len(networkServices)*3)
	for _, svc := range networkServices {
		cmds = append(cmds,
			[]string{"networksetup", "-setwebproxystate", svc, "off"},
			[]string{"networksetup", "-setsecurewebproxystate", svc, "off"},
			[]string{"networksetup", "-setautoproxyurl", svc, pacURL},
		)
	}

	if requiresAdminPrivileges() {
		if out, err := runElevated(cmds); err != nil {
			return fmt.Errorf("set system proxy with elevation: %v (%q)", err, out)
		}
		return nil
	}

	for _, args := range cmds {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil { // #nosec G204 -- command input comes from trusted sources
			if isAdminPrivilegesError(out) {
				// The pre-check reads system.preferences.network, but since macOS 26 networksetup
				// denies standard users regardless of that right (see discussion #751).
				// Retry the whole batch elevated; re-running already-succeeded commands is harmless.
				if out, err := runElevated(cmds); err != nil {
					return fmt.Errorf("set system proxy with elevation: %v (%q)", err, out)
				}
				return nil
			}
			return fmt.Errorf("%s: %v (%q)", strings.Join(args, " "), err, out)
		}
	}

	return nil
}

func unsetSystemProxy() error {
	if len(networkServices) == 0 {
		return nil
	}

	cmds := make([][]string, 0, len(networkServices))
	for _, svc := range networkServices {
		cmds = append(cmds, []string{"networksetup", "-setautoproxystate", svc, "off"})
	}

	var finalErr error
	if requiresAdminPrivileges() {
		if out, err := runElevated(cmds); err != nil {
			finalErr = fmt.Errorf("unset system proxy with elevation: %v (%q)", err, out)
		}
	} else {
		for _, args := range cmds {
			if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil { // #nosec G204 -- command input comes from trusted sources
				if isAdminPrivilegesError(out) {
					// See the equivalent fallback in setSystemProxy. The elevated batch retries
					// every command, superseding any per-command errors collected so far.
					if out, err := runElevated(cmds); err != nil {
						finalErr = fmt.Errorf("unset system proxy with elevation: %v (%q)", err, out)
					} else {
						finalErr = nil
					}
					break
				}
				finalErr = multierror.Append(finalErr, fmt.Errorf("%s: %v (%q)", strings.Join(args, " "), err, out))
			}
		}
	}

	networkServices = nil
	return finalErr
}

// requiresAdminPrivileges checks whether macOS requires admin privileges to modify network settings.
func requiresAdminPrivileges() bool {
	out, err := exec.Command("security", "authorizationdb", "read", "system.preferences.network").CombinedOutput()
	if err != nil {
		return false
	}

	var entry struct {
		Shared bool `plist:"shared"`
	}
	if _, err := plist.Unmarshal(out, &entry); err != nil {
		return false
	}
	// When "Require an administrator password to access system-wide settings" is enabled, "shared" is false.
	return !entry.Shared
}

// isAdminPrivilegesError reports whether networksetup output indicates the command
// was denied for lack of admin privileges ("** Error: Command requires admin privileges.",
// exit status 14). Matched on the message rather than the exit status, whose meaning
// is undocumented.
func isAdminPrivilegesError(out []byte) bool {
	return strings.Contains(strings.ToLower(string(out)), "requires admin privileges")
}

// discoverNetworkServices returns a list of all network service names.
func discoverNetworkServices() ([]string, error) {
	cmd := exec.Command("networksetup", "-listallnetworkservices")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list network services: %v (%q)", err, out)
	}

	lines := bytes.Split(out, []byte{'\n'})
	if len(lines) < 2 {
		return nil, errors.New("no network services found")
	}

	// The first line contains "An asterisk (*) denotes that a network service is disabled."
	services := make([]string, 0, len(lines)-1)
	for _, raw := range lines[1:] {
		line := strings.TrimSpace(string(raw))
		if line == "" {
			continue
		}
		if line[0] == '*' {
			// Disabled service; remove the asterisk.
			line = strings.TrimSpace(line[1:])
		}

		services = append(services, line)
	}

	return services, nil
}

// runElevated runs the given commands with administrator privileges via osascript (which sets real uid=0).
// The user will see a single macOS password prompt.
func runElevated(cmds [][]string) ([]byte, error) {
	parts := make([]string, len(cmds))
	for i, args := range cmds {
		quoted := make([]string, len(args))
		for j, a := range args {
			quoted[j] = shellQuote(a)
		}
		parts[i] = strings.Join(quoted, " ")
	}
	shellCmd := strings.Join(parts, "&&")
	script := fmt.Sprintf(`do shell script %s with administrator privileges with prompt "Authorize Zen to modify system proxy settings"`, appleScriptQuote(shellCmd))
	return exec.Command("osascript", "-e", script).CombinedOutput()
}

// shellQuote wraps s in single quotes so that sh treats every character in it literally.
// An embedded single quote is closed, escaped, and reopened. Network service names come from
// networksetup and may contain anything, including $(...) and backticks, which would otherwise
// be expanded by the shell inside "do shell script" - as root, on the elevated path.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var appleScriptEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

// appleScriptQuote wraps s in double quotes for AppleScript, where only \ and " are special.
// Go's %q is not a substitute: for non-printable runes it emits Go escapes (\u00XX, \a, \v)
// that AppleScript does not understand, which would corrupt the command.
func appleScriptQuote(s string) string {
	return `"` + appleScriptEscaper.Replace(s) + `"`
}
