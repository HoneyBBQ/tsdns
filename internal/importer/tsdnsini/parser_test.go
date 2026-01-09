package tsdnsini

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	input := `
# comment
public.teamspeak.com=12.13.14.15:10000
*.teamspeak-systems.de=1.2.3.4:15000
*=12.13.14.15:$PORT
voice.teamspeak.com=NORESPONSE
invalid-line-without-equals
`
	res, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if res.Skipped == 0 {
		t.Fatalf("expected some skipped lines")
	}
	if len(res.Entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(res.Entries))
	}
	if res.Entries[0].Ident != "public.teamspeak.com" || res.Entries[0].Value != "12.13.14.15:10000" {
		t.Fatalf("unexpected first entry: %+v", res.Entries[0])
	}
}
