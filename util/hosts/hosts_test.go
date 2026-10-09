package hosts

import (
	"strings"
	"testing"
)

func TestParseKeepsOrderAndDropsRepeats(t *testing.T) {
	got := Parse(" A.example.com, b.example.com\n*.Sub.Example.com. ; a.example.com\tc.example.com ")
	want := []string{"a.example.com", "b.example.com", "*.sub.example.com", "c.example.com"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("Parse = %q, want %q", got, want)
	}
}

func TestCheckList(t *testing.T) {
	for _, ok := range []string{"", "example.com", "a.example.com, *.sub.example.com", "1.2.3.4", "[2001:db8::1]", "x_y.example.com"} {
		if bad, err := CheckList(ok); err != nil {
			t.Errorf("CheckList(%q) = %q, %v", ok, bad, err)
		}
	}
	for list, bad := range map[string]string{
		"*.com":                     "*.com",
		"a.*.example.com":           "a.*.example.com",
		"ok.example.com, -bad.com":  "-bad.com",
		"*example.com":              "*example.com",
		"x.example.com, **.example": "**.example",
	} {
		got, err := CheckList(list)
		if err == nil || got != bad {
			t.Errorf("CheckList(%q) = %q, %v; want %q rejected", list, got, err, bad)
		}
	}
}

func TestMatchTakesExactlyOneLabel(t *testing.T) {
	entries := Parse("panel.example.com, *.sub.example.com, 10.0.0.1")
	cases := []struct {
		host, entry, label string
		ok                 bool
	}{
		{"panel.example.com", "panel.example.com", "", true},
		{"PANEL.example.com:443", "panel.example.com", "", true},
		{"abc.sub.example.com", "*.sub.example.com", "abc", true},
		{"abc.sub.example.com.", "*.sub.example.com", "abc", true},
		{"a.b.sub.example.com", "", "", false},
		{"sub.example.com", "", "", false},
		{"evil-sub.example.com", "", "", false},
		{"abc.sub.example.com.evil.net", "", "", false},
		{"10.0.0.1:2096", "10.0.0.1", "", true},
	}
	for _, c := range cases {
		entry, label, ok := Match(entries, c.host)
		if entry != c.entry || label != c.label || ok != c.ok {
			t.Errorf("Match(%q) = %q %q %v, want %q %q %v", c.host, entry, label, ok, c.entry, c.label, c.ok)
		}
	}
}

func TestLabelIsKeyedAndStable(t *testing.T) {
	a := Label([]byte("k1"), "sub.example.com", "ali")
	if a != Label([]byte("k1"), "SUB.example.com", "ali") {
		t.Fatal("the label changes with the case of the domain")
	}
	if len(a) != 10 || !validLabel(a) {
		t.Fatalf("label %q is not a 10 character DNS label", a)
	}
	for _, other := range []string{
		Label([]byte("k2"), "sub.example.com", "ali"),
		Label([]byte("k1"), "cdn.example.com", "ali"),
		Label([]byte("k1"), "sub.example.com", "Ali"),
	} {
		if other == a {
			t.Fatalf("labels collide: %q", a)
		}
	}
}

func TestConcrete(t *testing.T) {
	if got := Concrete([]byte("k"), "plain.example.com", "ali"); got != "plain.example.com" {
		t.Fatalf("plain entry became %q", got)
	}
	got := Concrete([]byte("k"), "*.sub.example.com", "ali")
	entry, label, ok := Match([]string{"*.sub.example.com"}, got)
	if !ok || entry != "*.sub.example.com" || label != Label([]byte("k"), "sub.example.com", "ali") {
		t.Fatalf("Concrete = %q does not match back: %q %q %v", got, entry, label, ok)
	}
}
