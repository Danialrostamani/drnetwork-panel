// Package hosts reads the domain lists the panel serves on: plain host names,
// IP literals and one-label wildcards such as "*.sub.example.com".
package hosts

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"net"
	"strings"
)

// Parse splits a list written with commas, spaces or new lines. Entries are
// lower-cased, trailing dots dropped and duplicates removed; the order is
// kept, since the first entry is the one new links use.
func Parse(list string) []string {
	fields := strings.FieldsFunc(list, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\r' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, f := range fields {
		f = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(f)), ".")
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// IsWildcard tells a "*.base" entry.
func IsWildcard(entry string) bool { return strings.HasPrefix(entry, "*.") }

// Base is the part of a wildcard entry under the star.
func Base(entry string) string { return strings.TrimPrefix(entry, "*.") }

func validLabel(l string) bool {
	if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
		return false
	}
	for _, r := range l {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// ValidName tells a host name made of DNS labels.
func ValidName(name string) bool {
	if name == "" || len(name) > 253 {
		return false
	}
	for _, l := range strings.Split(name, ".") {
		if !validLabel(l) {
			return false
		}
	}
	return true
}

// Check validates one entry of a list.
func Check(entry string) error {
	if IsWildcard(entry) {
		base := Base(entry)
		if !ValidName(base) || !strings.Contains(base, ".") {
			return errors.New("a wildcard must look like *.sub.example.com")
		}
		return nil
	}
	if strings.Contains(entry, "*") {
		return errors.New("a star may only stand alone as the first label: *.sub.example.com")
	}
	if net.ParseIP(strings.Trim(entry, "[]")) != nil {
		return nil
	}
	if !ValidName(entry) {
		return errors.New("not a valid host name")
	}
	return nil
}

// CheckList validates a whole list and returns the bad entry, if any.
func CheckList(list string) (string, error) {
	for _, e := range Parse(list) {
		if err := Check(e); err != nil {
			return e, err
		}
	}
	return "", nil
}

// Strip removes the port and brackets of a Host header and lower-cases it.
func Strip(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.TrimSuffix(strings.ToLower(strings.Trim(host, "[]")), ".")
}

// Match finds the entry a host belongs to. For a wildcard entry it also
// returns the label the host has in place of the star; exactly one label
// matches, so "a.b.sub.example.com" is not under "*.sub.example.com".
func Match(entries []string, host string) (entry, label string, ok bool) {
	host = Strip(host)
	for _, e := range entries {
		if !IsWildcard(e) && host == strings.Trim(e, "[]") {
			return e, "", true
		}
	}
	for _, e := range entries {
		if !IsWildcard(e) {
			continue
		}
		suffix := "." + Base(e)
		if l, found := strings.CutSuffix(host, suffix); found && validLabel(l) {
			return e, l, true
		}
	}
	return "", "", false
}

// Allowed tells whether a host is in the list.
func Allowed(entries []string, host string) bool {
	_, _, ok := Match(entries, host)
	return ok
}

// Label is the stable per-client name under a wildcard: a keyed hash of the
// domain and the client, so it cannot be guessed from the client's name and
// differs from one domain to the next.
func Label(secret []byte, base, name string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(strings.ToLower(base)))
	mac.Write([]byte{0})
	mac.Write([]byte(name))
	sum := mac.Sum(nil)
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum)
	return strings.ToLower(enc[:10])
}

// Concrete is the host a client gets for an entry: the entry itself, or the
// client's label in place of the star.
func Concrete(secret []byte, entry, name string) string {
	if !IsWildcard(entry) {
		return entry
	}
	return Label(secret, Base(entry), name) + "." + Base(entry)
}
