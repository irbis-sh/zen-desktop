package sysproxy

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// hostileArgs contains values that a network service could plausibly be named and that the
// shell, AppleScript, or both would interpret rather than pass through. The elevated path runs
// as root, so an expansion here would be a privilege escalation.
var hostileArgs = []string{
	"Wi-Fi",
	"$(id -un)",
	"`id -un`",
	"$HOME",
	"${HOME}",
	`quote"and\backslash`,
	"it's an apostrophe",
	"semicolon; echo pwned",
	"pipe | echo pwned",
	"and && echo pwned",
	"newline\nsecond line",
	"tab\there",
	"Wi‑Fi Éthernet",
}

func TestShellQuote(t *testing.T) {
	t.Parallel()

	for _, arg := range hostileArgs {
		t.Run(arg, func(t *testing.T) {
			t.Parallel()

			// printf writes its operand verbatim, so anything the shell expanded shows up as a diff.
			out, err := exec.Command("sh", "-c", "/usr/bin/printf %s "+shellQuote(arg)).Output() // #nosec G204 -- feeding tainted input to the shell is the point of the test
			if err != nil {
				t.Fatalf("run sh: %v", err)
			}

			if string(out) != arg {
				t.Fatalf("argument was altered by the shell: got %q, want %q", out, arg)
			}
		})
	}
}

func TestAppleScriptQuote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{`plain`, `"plain"`},
		{`with "quotes"`, `"with \"quotes\""`},
		{`with \backslash`, `"with \\backslash"`},
		{`$(id -un)`, `"$(id -un)"`}, // AppleScript does not expand these; the shell layer must.
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			if got := appleScriptQuote(tt.in); got != tt.want {
				t.Fatalf("appleScriptQuote(%q) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

// TestRunElevatedQuoting exercises the full quoting chain of runElevated - AppleScript string,
// then shell word - without actually elevating, by running the same osascript "do shell script"
// with a harmless command in place of networksetup.
func TestRunElevatedQuoting(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("osascript"); err != nil {
		t.Skipf("osascript unavailable: %v", err)
	}

	for _, arg := range hostileArgs {
		t.Run(arg, func(t *testing.T) {
			t.Parallel()

			args := []string{"/usr/bin/printf", "[%s]", arg}
			quoted := make([]string, len(args))
			for i, a := range args {
				quoted[i] = shellQuote(a)
			}
			script := fmt.Sprintf("do shell script %s", appleScriptQuote(strings.Join(quoted, " ")))

			out, err := exec.Command("osascript", "-e", script).CombinedOutput()
			if err != nil {
				t.Fatalf("run osascript: %v (%q)", err, out)
			}

			// do shell script returns the output with newlines translated to carriage returns.
			got := strings.ReplaceAll(strings.TrimSuffix(string(out), "\n"), "\r", "\n")
			if want := "[" + arg + "]"; got != want {
				t.Fatalf("argument was altered on the way to the command: got %q, want %q", got, want)
			}
		})
	}
}
