package terminal

import (
	"strings"
	"testing"
)

func TestCommandOutput(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe()})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	if _, ok := term.CommandOutput(0); ok {
		t.Error("output with no marks")
	}
	term.Resize(20, 6)
	prompt := "\x1b]133;A\a$ \x1b]133;B\a"
	run := func(cmd, out string) {
		term.Feed([]byte(prompt + cmd + "\r\n\x1b]133;C\a" + out + "\x1b]133;D;0\a"))
	}
	// Unmarked output before the first prompt is no command's.
	term.Feed([]byte("Last login\r\n"))
	run("ls", "a.go\r\nb.go\r\n")
	run("cd x", "")
	// A line longer than the row wraps, and more lines than the screen
	// holds push the start into the scrollback.
	long := strings.Repeat("y", 30)
	var b strings.Builder
	b.WriteString(long + "\r\n")
	for i := range 8 {
		b.WriteString("line " + string(rune('0'+i)) + "  \r\n")
	}
	run("cat log", b.String())
	term.Feed([]byte(prompt + "ech"))

	want := []string{long + "\nline 0\nline 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7", "", "a.go\nb.go"}
	for n, w := range want {
		if got, ok := term.CommandOutput(n); !ok || got != w {
			t.Errorf("CommandOutput(%d) = %q, %v, want %q", n, got, ok, w)
		}
	}
	if got, ok := term.CommandOutput(len(want)); ok {
		t.Errorf("output before the first prompt: %q", got)
	}

	// A command still running counts as the last.
	term.Feed([]byte("o hi\r\n\x1b]133;C\ahi\r\n"))
	if got, ok := term.CommandOutput(0); !ok || got != "hi" {
		t.Errorf("running command: %q, %v", got, ok)
	}

	term.Feed([]byte("\x1b[?1049h"))
	if _, ok := term.CommandOutput(0); ok {
		t.Error("output on the alternate screen")
	}
}
