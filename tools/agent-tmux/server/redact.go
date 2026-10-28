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

	// http(s)://TOKEN@host: a token as the whole userinfo, as GitHub tokens are often used. Only http(s), because
	// ssh://git@host names a user, not a secret.
	reURLToken = regexp.MustCompile(`(https?://)[^\s:/@'"]+@`)

	// --password VALUE, --password=VALUE and the same with a dashed prefix (--access-token, --auth-token,
	// --client-secret), in any case. A value may not start with '-', so "--pass -x" keeps the flag that follows.
	// An unquoted value runs on through backslash-escaped spaces (a\ b), as the shell reads it.
	reSecretLongFlag = regexp.MustCompile(`(?i)(^|\s)(--(?:[A-Za-z0-9]+-)*(?:password|passwd|pass|token|secret|api-key|apikey)(?:=|\s+))("[^"]*"|'[^']*'|[^\s\\-](?:\\.|[^\s\\])*)`)

	// curl -u user:pass, -u :token, --user user:pass and --user=user:pass: the part after the first ':'.
	reUserPass = regexp.MustCompile(`(^|\s)(-u\s*|--user(?:=|\s+))(["']?)([^\s:'"]*):[^\s'"]+`)

	// Authorization: <scheme> <credentials> in a header, whatever the scheme (token, Basic, Bearer).
	reAuthHeader = regexp.MustCompile(`(?i)(authorization:\s*[A-Za-z]+\s+)[^\s'"]+`)

	// Headers whose whole value is the key: X-Api-Key, Api-Key, X-Auth-Token, PRIVATE-TOKEN (GitLab).
	reKeyHeader = regexp.MustCompile(`(?i)\b((?:x-)?(?:api-key|auth-token)|private-token)(:\s*)[^\s'"]+`)

	// -p VALUE, -p=VALUE and -pVALUE (mysql style), lower case only: -P is a port in ssh, scp and mysql. This also
	// hits "mkdir -p dir" and "docker run -p 8080:80", which is accepted over-redaction.
	reSecretShortFlag = regexp.MustCompile(`(^|\s)(-p(?:=|\s*))("[^"]*"|'[^']*'|[^\s\\-](?:\\.|[^\s\\])*)`)

	// NAME=value where NAME ends in PASSWORD, PASSWD, PASS, PWD, PAT, TOKEN, SECRET or KEY in any case
	// (MYSQL_PWD, GITHUB_PAT). A quoted value may hold spaces. PWD and OLDPWD themselves are folders, kept.
	reSecretEnv = regexp.MustCompile(`(?i)\b([A-Za-z0-9_]*(?:PASSWORD|PASSWD|PASS|PWD|PAT|TOKEN|SECRET|KEY))=("[^"]*"|'[^']*'|(?:\\.|[^\s\\])+)`)

	reBearer = regexp.MustCompile(`(?i)\b(bearer\s+)[A-Za-z0-9._~+/=-]{6,}`)
)

// redact masks the secrets it recognises in text a tool echoes or logs.
func redact(s string) string {
	s = reURLUserinfo.ReplaceAllString(s, "${1}:***@")
	s = reURLToken.ReplaceAllString(s, "${1}***@")
	s = reSecretLongFlag.ReplaceAllString(s, "${1}${2}***")
	s = reUserPass.ReplaceAllString(s, "${1}${2}${3}${4}:***")
	s = reAuthHeader.ReplaceAllString(s, "${1}***")
	s = reKeyHeader.ReplaceAllString(s, "${1}${2}***")
	s = reSecretShortFlag.ReplaceAllString(s, "${1}${2}***")
	s = reSecretEnv.ReplaceAllStringFunc(s, func(m string) string {
		name := m[:strings.IndexByte(m, '=')]
		if u := strings.ToUpper(name); u == "PWD" || u == "OLDPWD" {
			return m
		}
		return name + "=***"
	})
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
