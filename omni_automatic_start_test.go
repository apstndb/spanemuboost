package spanemuboost

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Exercise the actual shell control flow with a fake Spanner executable. Live
// image startup, connectivity, and cleanup are covered by the Omni smoke tests.
func TestOmniAutomaticStartScript(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("automatic startup script requires a POSIX shell")
	}
	if _, err := exec.LookPath("grep"); err != nil {
		t.Skip("automatic startup script requires grep")
	}
	const legacyHelp = "Flags:\n  -h, --help   help for start-single-server\n"
	const ltsHelp = "Flags:\n      --listen-addresses string   addresses to listen on (default localhost)\n"
	const bindArg = "--listen-addresses=0.0.0.0"
	for _, tt := range []struct {
		name        string
		help        string
		args        []string
		want        []string
		helpStatus  int
		startStatus int
		wantStatus  int
		wantHelp    bool
	}{
		{name: "old image", help: legacyHelp, args: []string{"start-single-server"}, want: []string{"start-single-server"}, wantHelp: true},
		{name: "LTS image", help: ltsHelp, args: []string{"start-single-server"}, want: []string{"start-single-server", bindArg}, wantHelp: true},
		{name: "short alias", help: "  -l, --listen-addresses string  addresses\n", args: []string{"start-single-server"}, want: []string{"start-single-server", bindArg}, wantHelp: true},
		{name: "prose is not a flag", help: "  Use --listen-addresses on newer images.\n", args: []string{"start-single-server"}, want: []string{"start-single-server"}, wantHelp: true},
		{name: "similar flag is not supported", help: "      --listen-addresses-extra string\n", args: []string{"start-single-server"}, want: []string{"start-single-server"}, wantHelp: true},
		{name: "explicit equals wins", helpStatus: 17, args: []string{"start-single-server", "--listen-addresses=127.0.0.1"}, want: []string{"start-single-server", "--listen-addresses=127.0.0.1"}},
		{name: "explicit separated wins", helpStatus: 17, args: []string{"start-single-server", "--listen-addresses", "127.0.0.1"}, want: []string{"start-single-server", "--listen-addresses", "127.0.0.1"}},
		{name: "other command skips detection", helpStatus: 17, args: []string{"custom-subcommand", "custom-arg"}, want: []string{"custom-subcommand", "custom-arg"}},
		{name: "argv stays literal", help: ltsHelp, args: []string{"start-single-server", "a b;$(false)'quoted'"}, want: []string{"start-single-server", "a b;$(false)'quoted'", bindArg}, wantHelp: true},
		{name: "help failure aborts startup", help: ltsHelp, helpStatus: 17, args: []string{"start-single-server"}, wantStatus: 17, wantHelp: true},
		{name: "start status propagates", help: legacyHelp, startStatus: 23, args: []string{"start-single-server"}, want: []string{"start-single-server"}, wantStatus: 23, wantHelp: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			fake := filepath.Join(dir, "spanner")
			argvFile := filepath.Join(dir, "argv")
			helpFile := filepath.Join(dir, "help-called")
			if err := os.WriteFile(fake, []byte(`#!/bin/sh
if [ "${1:-}" = start-single-server ] && [ "${2:-}" = --help ]; then
    printf called > "$TEST_OMNI_HELP_FILE"
    printf '%s' "$TEST_OMNI_HELP"
    exit "$TEST_OMNI_HELP_STATUS"
fi
printf '%s\000' "$@" > "$TEST_OMNI_ARGV_FILE"
exit "$TEST_OMNI_START_STATUS"
`), 0o700); err != nil {
				t.Fatal(err)
			}
			quotedFake := "'" + strings.ReplaceAll(fake, "'", "'\"'\"'") + "'"
			script := strings.Replace(omniAutomaticStartScript, "spanner=/google/spanner/bin/spanner", "spanner="+quotedFake, 1)
			args := append([]string{"-c", script, "--", "spanemuboost-omni"}, tt.args...)
			cmd := exec.CommandContext(t.Context(), "/bin/sh", args...)
			cmd.Env = append(os.Environ(),
				"TEST_OMNI_HELP="+tt.help,
				fmt.Sprintf("TEST_OMNI_HELP_STATUS=%d", tt.helpStatus),
				fmt.Sprintf("TEST_OMNI_START_STATUS=%d", tt.startStatus),
				"TEST_OMNI_HELP_FILE="+helpFile,
				"TEST_OMNI_ARGV_FILE="+argvFile,
			)
			output, err := cmd.CombinedOutput()
			status := 0
			if err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatal(err)
				}
				status = exitErr.ExitCode()
			}
			if status != tt.wantStatus {
				t.Fatalf("exit status = %d, want %d; output: %s", status, tt.wantStatus, output)
			}
			_, err = os.Stat(helpFile)
			if tt.wantHelp && err != nil || !tt.wantHelp && !os.IsNotExist(err) {
				t.Fatalf("help invocation: stat error = %v, wantHelp = %t", err, tt.wantHelp)
			}
			got, err := os.ReadFile(argvFile)
			if tt.want == nil {
				if !os.IsNotExist(err) {
					t.Fatalf("startup ran after help failure: argv = %q, error = %v", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			argv := strings.Split(strings.TrimSuffix(string(got), "\x00"), "\x00")
			if !slices.Equal(argv, tt.want) {
				t.Fatalf("argv = %q, want %q", argv, tt.want)
			}
		})
	}
}
