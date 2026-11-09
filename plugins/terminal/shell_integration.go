package terminal

import (
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Ghostty's C API preserves prompts by default. zsh instead redraws the
// whole prompt on SIGWINCH; the final C leaves the initial screen unmarked.
const promptRedraw = "\x1b]133;A;redraw=1\a\x1b]133;C\a"

// ZshIntegration returns env for a zsh that marks its prompts and commands
// with OSC 133, as Options.ShellIntegration has a local zsh do, for a
// program that starts the shell itself and connects it with Conn. Its user
// startup files are sourced as they would be. dir holds the startup file
// that installs the marks, for the caller to remove once the shell ended.
func ZshIntegration(env []string) (_ []string, dir string, _ error) { return zshEnvironment(env) }

func zshEnvironment(env []string) ([]string, string, error) {
	dir, err := os.MkdirTemp("", "mygo-zsh-")
	if err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(filepath.Join(dir, ".zshenv"), []byte(zshIntegration), 0o600); err != nil {
		os.RemoveAll(dir)
		return nil, "", err
	}
	clean := make([]string, 0, len(env)+1)
	var original string
	var hadOriginal bool
	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, "ZDOTDIR="); ok {
			original, hadOriginal = value, true
		} else if !strings.HasPrefix(entry, "MYGO_TERMINAL_ZDOTDIR=") {
			clean = append(clean, entry)
		}
	}
	if hadOriginal {
		clean = append(clean, "MYGO_TERMINAL_ZDOTDIR="+original)
	}
	return append(clean, "ZDOTDIR="+dir), dir, nil
}

// Restore ZDOTDIR before sourcing any user configuration. Install hooks at
// the first precmd, after startup files and prompt themes have loaded. P10k
// emits its own markers when asked, including asynchronous prompt updates.
const zshIntegration = `if (( ${+MYGO_TERMINAL_ZDOTDIR} )); then
  export ZDOTDIR=$MYGO_TERMINAL_ZDOTDIR
  unset MYGO_TERMINAL_ZDOTDIR
else
  unset ZDOTDIR
fi
[[ ! -r ${ZDOTDIR-$HOME}/.zshenv ]] || source -- "${ZDOTDIR-$HOME}/.zshenv"
[[ -o interactive ]] || return

typeset -gi __p9k_force_term_shell_integration=1

_mygo_mark_prompt() {
  local code=$?
  emulate -L zsh
  if ! zle; then
    printf '\e]133;D;%d\a' $code
  fi
  [[ -o promptpercent ]] || return
  local start=$'%{\e]133;A;cl=line;redraw=1\a%}' input=$'%{\e]133;B\a%}'
  local continuation=$'%{\e]133;P;k=s\a%}'
  if [[ $PS1 != *$'\e]133;'* ]]; then
    PS1=$start${PS1//$'\n'/$'\n'$continuation}$input
  fi
  [[ $PS2 == *$'\e]133;'* ]] || PS2=$continuation$PS2$input
}
_mygo_encode() {
  emulate -L zsh
  local LC_ALL=C s=$1 c out= i
  for (( i = 1; i <= ${#s}; i++ )); do
    c=${s[i]}
    if [[ $c == [A-Za-z0-9_.~/:=+@,-] ]]; then
      out+=$c
    else
      out+=%${(l:2::0:)$(( [##16] #c ))}
    fi
  done
  REPLY=$out
}
_mygo_command_start() {
  emulate -L zsh
  local REPLY
  # A line typed with a space first is one the user keeps out of history.
  if [[ $1 == ' '* || ${#1} -gt $_mygo_input_max ]]; then
    printf '\e]133;C\a'
    return
  fi
  _mygo_encode "$1"
  printf '\e]133;C;cmdline_url=%s\a' "$REPLY"
}
typeset -g _mygo_input=
typeset -gi _mygo_input_max=2000
_mygo_report_input() {
  emulate -L zsh
  local now="$CURSOR;$BUFFER" REPLY
  [[ $now == "$_mygo_input" ]] && return
  _mygo_input=$now
  if (( ${#BUFFER} > _mygo_input_max )); then
    print -rn -- $'\e]7733;input;-1;\a' >/dev/tty
    return
  fi
  _mygo_encode "$BUFFER"
  print -rn -- $'\e]7733;input;'"$CURSOR;$REPLY"$'\a' >/dev/tty
}
_mygo_line_init() {
  _mygo_input=
  _mygo_report_input
}
_mygo_clear_input() {
  BUFFER=
  CURSOR=0
}
_mygo_install_marks() {
  emulate -L zsh
  autoload -Uz add-zsh-hook
  add-zsh-hook -d precmd _mygo_install_marks
  add-zsh-hook precmd _mygo_mark_prompt
  add-zsh-hook preexec _mygo_command_start
  if autoload -Uz add-zle-hook-widget 2>/dev/null; then
    zle -N _mygo_report_input
    zle -N _mygo_line_init
    zle -N _mygo_clear_input
    add-zle-hook-widget line-init _mygo_line_init
    add-zle-hook-widget line-pre-redraw _mygo_report_input
    local m
    for m in emacs viins vicmd; do
      bindkey -M $m $'\e[7733~' _mygo_clear_input
    done
  fi
  _mygo_mark_prompt
}
typeset -ga precmd_functions
precmd_functions+=(_mygo_install_marks)
`

// Track only OSC 133, across read boundaries. Other control strings are
// opaque and OSC payloads are bounded; a TUI's text must not mark a prompt.
// The command line a command starts with (133;C;cmdline_url=) and what is
// typed at the prompt (the integration's 7733;input) may be longer.
type promptTracker struct {
	state     byte
	body      []byte
	length    int
	truncated bool
	atPrompt  bool

	// reports is whether the shell tells what is typed at its prompt:
	// input and cursor, in characters, are what it last told.
	reports bool
	known   bool
	input   string
	cursor  int
	// started are the command lines of commands that started, and ended
	// the statuses of those that ended, since the terminal last took them.
	started []string
	ended   []int
}

// markLimit is how long an OSC 133 may be, and lineLimit the command
// line or input marks, as the integration sends them encoded.
const (
	markLimit = 64
	lineLimit = 8192
)

func (p *promptTracker) limit() int {
	if b := p.body; bytesHasPrefix(b, "133;C;") || bytesHasPrefix(b, "7733;") {
		return lineLimit
	}
	return markLimit
}

func bytesHasPrefix(b []byte, s string) bool {
	return len(b) >= len(s) && string(b[:len(s)]) == s
}

func (p *promptTracker) feed(data []byte) {
	for _, b := range data {
		if b == 0x18 || b == 0x1a { // CAN / SUB
			p.state, p.length = 0, 0
			p.body = p.body[:0]
			continue
		}
		switch p.state {
		case 0:
			if b == 0x1b {
				p.state = 1
			}
		case 1:
			p.escape(b)
		case 2:
			p.length++
			switch b {
			case 7:
				p.finish()
				p.state = 0
			case 0x1b:
				p.state = 3
			default:
				if len(p.body) < p.limit() {
					p.body = append(p.body, b)
				} else {
					p.truncated = true
				}
			}
		case 3:
			if b == '\\' {
				p.finish()
				p.state = 0
			} else {
				p.escape(b) // ESC aborted the previous OSC.
			}
		case 4:
			if b == 0x1b {
				p.state = 5
			}
		case 5:
			if b == '\\' {
				p.state = 0
			} else if b != 0x1b {
				p.state = 4
			}
		}
	}
}

func (p *promptTracker) escape(b byte) {
	p.state = 0
	switch b {
	case ']':
		p.state, p.length, p.body, p.truncated = 2, 0, p.body[:0], false
	case 'P', '_', '^', 'X': // DCS, APC, PM, SOS
		p.state = 4
	case 'c': // RIS
		p.atPrompt = false
	case 0x1b:
		p.state = 1
	}
}

func (p *promptTracker) finish() {
	if p.truncated {
		return
	}
	body := string(p.body)
	if in, ok := strings.CutPrefix(body, "7733;input;"); ok {
		p.reported(in)
		return
	}
	mark, ok := strings.CutPrefix(body, "133;")
	if !ok || mark == "" || len(mark) > 1 && mark[1] != ';' {
		return
	}
	switch mark[0] {
	case 'A', 'B', 'I', 'N', 'P':
		p.atPrompt = true
	case 'C':
		p.atPrompt = false
		p.input, p.cursor = "", 0
		if line, ok := markParam(mark, "cmdline_url"); ok {
			if s, err := url.PathUnescape(line); err == nil && strings.TrimSpace(s) != "" && len(p.started) < 16 {
				p.started = append(p.started, s)
			}
		}
	case 'D':
		p.atPrompt = false
		if len(mark) > 2 && len(p.ended) < 16 {
			code, _, _ := strings.Cut(mark[2:], ";")
			if n, err := strconv.Atoi(code); err == nil {
				p.ended = append(p.ended, n)
			}
		}
	}
}

// reported takes what the shell says is typed at its prompt:
// "cursor;encoded input", or a cursor of -1 for input too long to say.
func (p *promptTracker) reported(s string) {
	cur, enc, ok := strings.Cut(s, ";")
	n, err := strconv.Atoi(cur)
	if !ok || err != nil {
		return
	}
	p.reports = true
	if n < 0 {
		p.known, p.input, p.cursor = false, "", 0
		return
	}
	text, err := url.PathUnescape(enc)
	if err != nil {
		return
	}
	p.known, p.input, p.cursor = true, text, min(n, len([]rune(text)))
}

// markParam returns the value of key in a mark's parameters, as
// "C;key=value;other=…".
func markParam(mark, key string) (string, bool) {
	for _, kv := range strings.Split(mark, ";")[1:] {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			return v, true
		}
	}
	return "", false
}
