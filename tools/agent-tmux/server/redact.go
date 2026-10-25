package main

import (
	"regexp"
	"strings"
)

// privateMarker stands in for text that starts with a space. That is the shell's own "do not record this"
// convention, so agent-tmux never writes such a command anywhere.
const privateMarker = "[private]"

// The rules below are best effort. They catch the common ways a secret sits on a command line and miss the
// rest: a bare token as an argument, a password in a here-doc, a secret split across two sends. The leading
// space and not typing secrets at all are the dependable controls.
var (
	// scheme://user:pass@host. The password runs to the last '@' before a '/', space or quote, so a '@' inside it
	// is covered too. A password holding a raw '/' is not valid in a URL (it must be percent-encoded) and is not caught.
	reURLUserinfo = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://[^\s:/@'"]*):[^\s'"/]*@`)

	// --password VALUE, --password=VALUE and a few like it, in any case. A value may not start with '-', so
	// "--pass -x" keeps the flag that follows.
	reSecretLongFlag = regexp.MustCompile(`(?i)(^|\s)(--(?:password|passwd|pass|token|secret|api-key|apikey)(?:=|\s+))("[^"]*"|'[^']*'|[^\s-]\S*)`)

	// -p VALUE, -p=VALUE and -pVALUE (mysql style), lower case only: -P is a port in ssh, scp and mysql. This also
	// hits "mkdir -p dir" and "docker run -p 8080:80", which is accepted over-redaction.
	reSecretShortFlag = regexp.MustCompile(`(^|\s)(-p(?:=|\s*))("[^"]*"|'[^']*'|[^\s-]\S*)`)

	// NAME=value where NAME ends in PASSWORD, TOKEN, SECRET or KEY in any case. A quoted value may hold spaces.
	reSecretEnv = regexp.MustCompile(`(?i)\b([A-Za-z0-9_]*(?:PASSWORD|TOKEN|SECRET|KEY))=("[^"]*"|'[^']*'|\S+)`)

	reBearer = regexp.MustCompile(`(?i)\b(bearer\s+)[A-Za-z0-9._~+/=-]{6,}`)
)

// redact masks the secrets it recognises in text a tool echoes or logs.
func redact(s string) string {
	s = reURLUserinfo.ReplaceAllString(s, "${1}:***@")
	s = reSecretLongFlag.ReplaceAllString(s, "${1}${2}***")
	s = reSecretShortFlag.ReplaceAllString(s, "${1}${2}***")
	s = reSecretEnv.ReplaceAllString(s, "${1}=***")
	return reBearer.ReplaceAllString(s, "${1}***")
}

// redactCmd is redact for a command agent-tmux typed: a leading space means the human or agent does not want it recorded.
func redactCmd(s string) string {
	if strings.HasPrefix(s, " ") {
		return privateMarker
	}
	return redact(s)
}

// fixedText makes a name that tmux or a program chose safe for an event or a reply. Events reach a watcher
// unwrapped, so window and session names get a character whitelist and a length cap, not only neutralise. A name
// the agent chose is also redacted, as everything else a record echoes is.
func fixedText(s string) string {
	b := make([]rune, 0, maxNameLen)
	for _, r := range redact(s) {
		if len(b) == maxNameLen {
			break
		}
		if r < 0x20 || r > 0x7e || r == '#' || r == '"' || r == '\\' {
			r = '?'
		}
		b = append(b, r)
	}
	return neutralise(string(b))
}
