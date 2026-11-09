package terminal

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/internal/wait"
)

// The tracker takes the command lines, statuses and input the integration
// marks, encoded and in pieces, and refuses what is too long or malformed.
func TestPromptTrackerCommandsAndInput(t *testing.T) {
	var p promptTracker
	for _, part := range []string{"\x1b]133;A\a\x1b]133;B\a\x1b]7733;inp", "ut;3;git%20st\a"} {
		p.feed([]byte(part))
	}
	if !p.reports || !p.known || p.input != "git st" || p.cursor != 3 {
		t.Fatalf("input = %q at %d (known %v)", p.input, p.cursor, p.known)
	}
	p.feed([]byte("\x1b]133;C;cmdline_url=git%20status%3B%20echo%20%E2%9C%93\x1b\\"))
	if p.atPrompt || p.input != "" || len(p.started) != 1 || p.started[0] != "git status; echo ✓" {
		t.Fatalf("started = %q, input %q, at prompt %v", p.started, p.input, p.atPrompt)
	}
	p.feed([]byte("\x1b]133;D;127\a\x1b]133;D\a"))
	if len(p.ended) != 1 || p.ended[0] != 127 {
		t.Fatalf("ended = %v", p.ended)
	}
	p.feed([]byte("\x1b]7733;input;-1;\a"))
	if p.known || !p.reports {
		t.Fatal("input too long to tell was taken as known")
	}
	p.feed([]byte("\x1b]7733;input;" + strings.Repeat("x", lineLimit+10) + "\a"))
	if p.known {
		t.Fatal("oversized input was accepted")
	}
	p.feed([]byte("\x1b]7733;input;x;y\a\x1b]133;C;cmdline_url=%zz\a"))
	if p.known || len(p.started) != 1 {
		t.Fatal("malformed marks were accepted")
	}
	p.feed([]byte("\x1b]133;C;cmdline_url=%20%20\a"))
	if len(p.started) != 1 {
		t.Fatal("a blank command line was told")
	}
}

// A real zsh tells what is typed, the commands it runs, and how they end,
// and the integration's key empties the line for ReplaceInput.
func TestShellIntegrationZshInput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a Unix zsh PTY")
	}
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is not installed")
	}
	loadLib(t)
	home := t.TempDir()
	if err := os.WriteFile(home+"/.zshrc", []byte("PROMPT='%# '\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", zsh)
	t.Setenv("HOME", home)
	var mu sync.Mutex
	var started []string
	var ended []int
	term, err := New(Options{
		ShellIntegration: true,
		OnCommand:        func(l string) { mu.Lock(); started = append(started, l); mu.Unlock() },
		OnCommandEnd:     func(c int) { mu.Lock(); ended = append(ended, c); mu.Unlock() },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	term.Resize(80, 24)
	wait.Until(t, "prompt", term.AtPrompt)
	term.Send([]byte("echo héllo wörld"))
	wait.Until(t, "typed input", func() bool {
		s, c, ok := term.PromptInput()
		return ok && s == "echo héllo wörld" && c == len([]rune(s))
	})
	term.Send([]byte("\x1b[D\x1b[D"))
	wait.Until(t, "cursor moved", func() bool {
		_, c, ok := term.PromptInput()
		return ok && c == len([]rune("echo héllo wörld"))-2
	})
	term.ReplaceInput("printf 'replaced\\n'; false")
	wait.Until(t, "replaced input", func() bool {
		s, _, ok := term.PromptInput()
		return ok && s == "printf 'replaced\\n'; false"
	})
	term.Send([]byte("\r"))
	wait.Until(t, "command told", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(started) == 1 && len(ended) >= 1 && ended[len(ended)-1] == 1
	})
	mu.Lock()
	if started[0] != "printf 'replaced\\n'; false" {
		t.Errorf("command line = %q", started[0])
	}
	mu.Unlock()
	wait.Until(t, "prompt again", func() bool {
		s, _, ok := term.PromptInput()
		return ok && s == ""
	})
	if strings.Contains(term.Text(), "héllo") && !strings.Contains(term.Text(), "replaced") {
		t.Fatalf("screen = %q", term.Text())
	}
	term.Send([]byte(" echo secret\r"))
	wait.Until(t, "secret ran", func() bool { return strings.Count(term.Text(), "secret") >= 2 })
	mu.Lock()
	if len(started) != 1 {
		t.Errorf("a line typed with a space first was told: %q", started)
	}
	mu.Unlock()
	// zsh writes its history into home as it exits.
	term.Close()
	wait.For(t, "zsh exit", term.Done())
}
