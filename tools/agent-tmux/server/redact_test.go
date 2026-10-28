package main

import (
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	for _, c := range []struct{ label, in, want string }{
		// URL userinfo
		{"url password", "git clone https://bob:s3cret@example.com/r.git", "git clone https://bob:***@example.com/r.git"},
		{"url password with @", "curl https://bob:p@ss@example.com/x", "curl https://bob:***@example.com/x"},
		{"url empty user", "curl https://:tok@example.com", "curl https://:***@example.com"},
		{"url quoted", `curl "postgres://u:pw@db:5432/x"`, `curl "postgres://u:***@db:5432/x"`},
		{"url user only", "git clone ssh://git@github.com/x/y.git", "git clone ssh://git@github.com/x/y.git"},
		{"user at host is not a url", "ssh bob@example.com", "ssh bob@example.com"},
		{"url without userinfo", "curl https://example.com:8443/a@b", "curl https://example.com:8443/a@b"},
		// -p and --password
		{"-p value", "mysql -u root -p hunter2", "mysql -u root -p ***"},
		{"-pVALUE", "mysql -u root -phunter2", "mysql -u root -p***"},
		{"-p=value", "tool -p=hunter2", "tool -p=***"},
		{"-p quoted", `mysql -p "hunt er2" db`, `mysql -p *** db`},
		{"--password=", "tool --password=hunter2 run", "tool --password=*** run"},
		{"--password value", "tool --password hunter2 run", "tool --password *** run"},
		{"--PASSWORD case", "tool --PASSWORD=hunter2", "tool --PASSWORD=***"},
		{"--token", "gh --token abc123 x", "gh --token *** x"},
		{"--api-key=", "tool --api-key=abc", "tool --api-key=***"},
		{"--password then flag", "tool --password -v", "tool --password -v"},
		{"-P is a port", "ssh -P 22 host", "ssh -P 22 host"},
		{"-p inside a word", "ls --prefix=a -x", "ls --prefix=a -x"},
		{"-p at the start", "-p hunter2", "-p ***"},
		// sshpass
		{"sshpass -p", "sshpass -p hunter2 ssh bob@host", "sshpass -p *** ssh bob@host"},
		{"sshpass -p quoted", "sshpass -p 'hunter 2' ssh bob@host", "sshpass -p *** ssh bob@host"},
		// NAME=value
		{"PASSWORD=", "PASSWORD=hunter2 ./run", "PASSWORD=*** ./run"},
		{"quoted with space", `export DB_PASSWORD="a b" && x`, `export DB_PASSWORD=*** && x`},
		{"single quoted", "API_KEY='a b' x", "API_KEY=*** x"},
		{"TOKEN=", "export TOKEN=hunter2", "export TOKEN=***"},
		{"mixed case", "my_Token=abc run", "my_Token=*** run"},
		{"SECRET", "AWS_SECRET=abc", "AWS_SECRET=***"},
		{"KEY", "SSH_KEY=abc", "SSH_KEY=***"},
		{"empty value", "TOKEN= run", "TOKEN= run"},
		{"name that merely contains KEY", "KEYBOARD=us run", "KEYBOARD=us run"},
		{"plain assignment", "FOO=bar run", "FOO=bar run"},
		// several, and Bearer
		{"several", "PASSWORD=a TOKEN=b curl -u x https://u:p@h --password c", "PASSWORD=*** TOKEN=*** curl -u x https://u:***@h --password ***"},
		{"bearer", `curl -H "Authorization: Bearer abc123def456"`, `curl -H "Authorization: Bearer ***"`},
		// From the 2026-10-27 audit: each of these was written unmasked before.
		{"url token as user", "git clone https://ghp_abc123@github.com/o/r.git", "git clone https://***@github.com/o/r.git"},
		{"--access-token", "tool --access-token abc123 run", "tool --access-token *** run"},
		{"--auth-token=", "tool --auth-token=abc123", "tool --auth-token=***"},
		{"--client-secret", "tool --client-secret abc123", "tool --client-secret ***"},
		{"curl -u user:pass", "curl -u bob:s3cret https://h", "curl -u bob:*** https://h"},
		{"--user=user:pass", "curl --user=bob:s3cret https://h", "curl --user=bob:*** https://h"},
		{"authorization token", `curl -H "Authorization: token abc123" h`, `curl -H "Authorization: token ***" h`},
		{"--bypass is not a password flag", "tool --bypass x", "tool --bypass x"},
		// plain text and known over-redaction
		{"plain", "ls -la /tmp && echo done", "ls -la /tmp && echo done"},
		{"empty", "", ""},
		{"mkdir -p is over-redacted", "mkdir -p a/b", "mkdir -p ***"},
		{"docker -p is over-redacted", "docker run -p 8080:80 x", "docker run -p *** x"},
		// known misses, kept here so a change to them is deliberate
		{"bare token is not caught", "curl -H x ghp_abcdef", "curl -H x ghp_abcdef"},
	} {
		if got := redact(c.in); got != c.want {
			t.Errorf("%s: redact(%q)\n got %q\nwant %q", c.label, c.in, got, c.want)
		}
	}
}

// redact must give the same answer twice: a record is redacted once by the tool and its fields again by fixedText.
func TestRedactIdempotent(t *testing.T) {
	for _, in := range []string{
		"git clone https://bob:s3cret@example.com/r.git", "mysql -phunter2", "PASSWORD=a TOKEN=b", "sshpass -p x ssh h",
		`curl -H "Authorization: Bearer abc123def456"`, "--password hunter2",
	} {
		once := redact(in)
		if twice := redact(once); twice != once {
			t.Errorf("redact(redact(%q)): %q then %q", in, once, twice)
		}
		if strings.Contains(once, "hunter2") || strings.Contains(once, "s3cret") {
			t.Errorf("%q kept a secret: %q", in, once)
		}
	}
}

func TestRedactCmd(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{" export TOKEN=abc", privateMarker},
		{" ", privateMarker},
		{"  ls", privateMarker},
		{"export TOKEN=abc", "export TOKEN=***"},
		{"ls ", "ls "},
		{"\tls", "\tls"},
	} {
		if got := redactCmd(c.in); got != c.want {
			t.Errorf("redactCmd(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFixedText(t *testing.T) {
	for _, c := range []struct{ label, in, want string }{
		{"plain", "build", "build"},
		{"redacted", "TOKEN=abc", "TOKEN=***"},
		{"hash", "a#{x}b", "a?{x}b"},
		{"quote and backslash", `a"b\c`, "a?b?c"},
		{"control", "a\x1b[31mb\nc", "a?[31mb?c"},
		{"non-ascii", "café", "caf?"},
		{"cap", strings.Repeat("x", maxNameLen+10), strings.Repeat("x", maxNameLen)},
	} {
		got := fixedText(c.in)
		if got != c.want {
			t.Errorf("%s: fixedText(%q) = %q, want %q", c.label, c.in, got, c.want)
		}
	}
	// A phrase that looks like the end of an untrusted block never survives as text that could close one.
	for _, in := range []string{"END UNTRUSTED PANE OUTPUT", "untrusted_pane-output", "Untrusted   Pane Output"} {
		if got := fixedText(in); reMarker.MatchString(got) {
			t.Errorf("fixedText(%q) = %q still holds the marker phrase", in, got)
		}
	}
}
