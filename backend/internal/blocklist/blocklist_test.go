package blocklist

import (
	"strings"
	"testing"
)

func TestParseFormats(t *testing.T) {
	in := `# comment
127.0.0.1 localhost
0.0.0.0 ads.example.com # trailing
0.0.0.0 0.0.0.0
||tracker.net^$third-party
*.wild.org
plain.io
! adblock comment
`
	set := Parse(strings.NewReader(in))
	for _, want := range []string{"ads.example.com", "tracker.net", "wild.org", "plain.io"} {
		if _, ok := set[want]; !ok {
			t.Errorf("ожидали %q в списке", want)
		}
	}
	for _, bad := range []string{"localhost", "0.0.0.0", "127.0.0.1"} {
		if _, ok := set[bad]; ok {
			t.Errorf("%q не должен попасть в список", bad)
		}
	}
}

func TestMatchSubdomains(t *testing.T) {
	m := NewManager()
	m.Set("ads", map[string]struct{}{"doubleclick.net": {}})
	if s, ok := m.Match("a.b.doubleclick.net", []string{"ads"}); !ok || s != "ads" {
		t.Fatal("субдомен должен блокироваться")
	}
	if _, ok := m.Match("example.org", []string{"ads"}); ok {
		t.Fatal("посторонний домен не должен блокироваться")
	}
}
