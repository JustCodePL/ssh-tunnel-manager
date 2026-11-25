package sshconfig

import (
	"bufio"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRemoteDesktopLineRoundTrip(t *testing.T) {
	entries := []RemoteDesktopEntry{
		{Name: "laptop", Address: "192.168.1.20", Port: 3389, Protocol: "rdp"},
		{Name: "work laptop", Address: "laptop.corp", Port: 3390, Protocol: "rdp", Group: "Office team", Color: "#ff8800", Pinned: true},
		{Name: `odd "name" \ here`, Address: "fd00::1", Port: 5901, Protocol: "vnc", Group: `a\b`},
	}
	for _, want := range entries {
		line := RenderRemoteDesktopLine(want)
		got, ok := ParseRemoteDesktopLine(line)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Errorf("%q parsed as %+v %v, want %+v", line, got, ok, want)
		}
	}
	line := RenderRemoteDesktopLine(entries[1])
	want := `# stm:rd="work laptop" address=laptop.corp port=3390 protocol=rdp group="Office team" color=#ff8800 pinned=true`
	if line != want {
		t.Errorf("line = %q\nwant  %q", line, want)
	}
}

func TestRemoteDesktopLineRejectsBrokenEntries(t *testing.T) {
	for _, line := range []string{
		"# stm:group=x",
		"# stm:rd= address=a protocol=rdp",
		"# stm:rd=a protocol=rdp",
		"# stm:rd=a address=b protocol=ssh",
		`# stm:rd="unterminated address=b protocol=rdp`,
		"# stm:rd=a address=b protocol=rdp junk",
	} {
		if entry, ok := ParseRemoteDesktopLine(line); ok {
			t.Errorf("%q accepted as %+v", line, entry)
		}
	}
	// A missing or invalid port falls back to the protocol's standard one.
	for line, port := range map[string]int{
		"# stm:rd=a address=b protocol=rdp":           3389,
		"# stm:rd=a address=b protocol=vnc port=0":    5900,
		"# stm:rd=a address=b protocol=vnc port=5901": 5901,
	} {
		if entry, ok := ParseRemoteDesktopLine(line); !ok || entry.Port != port {
			t.Errorf("%q = %+v %v", line, entry, ok)
		}
	}
}

const configWithSection = `# stm:group=Office
Host jump
    HostName jump.example
    User me

# stm:remote-desktops-begin
# stm:rd=laptop address=10.0.0.5 port=3389 protocol=rdp group=Office
# stm:rd=mac address=mac.local port=5900 protocol=vnc
# stm:remote-desktops-end
# stm:pinned=true
Host after
    HostName after.example
`

func lines(s string) []string { return strings.Split(strings.TrimRight(s, "\n"), "\n") }

func TestParseSkipsTheSection(t *testing.T) {
	entries, err := Parse(bufio.NewScanner(strings.NewReader(configWithSection)))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Alias != "jump" || entries[1].Alias != "after" {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].Group != "Office" || entries[0].Pinned || !entries[1].Pinned || entries[1].Group != "" {
		t.Fatalf("metadata leaked: %+v", entries)
	}
	desktops := ParseRemoteDesktops(lines(configWithSection))
	if len(desktops) != 2 || desktops[0].Name != "laptop" || desktops[1].Protocol != "vnc" {
		t.Fatalf("desktops = %+v", desktops)
	}
}

func TestParseEndsAHostBlockAtTheSection(t *testing.T) {
	config := "Host a\n    HostName a.example\n# stm:remote-desktops-begin\n# stm:rd=x address=y protocol=rdp\n# stm:remote-desktops-end\n    User stray\n"
	entries, err := Parse(bufio.NewScanner(strings.NewReader(config)))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].User != "" {
		t.Fatalf("entries = %+v", entries)
	}
}

// Versions before the section read its lines as stm metadata: every line of
// the section is an unknown "# stm:" key and must change nothing.
func TestOlderVersionsIgnoreTheSectionLines(t *testing.T) {
	for _, line := range lines(configWithSection)[5:9] {
		key, _ := parseMetaComment(line)
		switch key {
		case "", "rd":
		default:
			t.Errorf("%q has key %q an older parser knows", line, key)
		}
	}
}

func TestFindHostBlockLeavesTheSectionAlone(t *testing.T) {
	all := lines(configWithSection)
	jump := FindHostBlock(all, "jump")
	if jump == nil || jump.StartLine != 0 || jump.EndLine != 4 {
		t.Fatalf("jump = %+v", jump)
	}
	after := FindHostBlock(all, "after")
	if after == nil || after.StartLine != 9 {
		t.Fatalf("after = %+v", after)
	}
	replaced, err := ReplaceHostBlock(all, "jump", "Host jump\n    HostName new.example\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := ParseRemoteDesktops(replaced); len(got) != 2 {
		t.Fatalf("section lost after replace: %q", replaced)
	}
	removed, err := RemoveHostBlock(all, "after")
	if err != nil {
		t.Fatal(err)
	}
	if got := ParseRemoteDesktops(removed); len(got) != 2 || removed[len(removed)-1] != RemoteDesktopsEnd {
		t.Fatalf("section lost after remove: %q", removed)
	}
}

func TestReplaceRemoteDesktopsSection(t *testing.T) {
	host := []string{"Host a", "    HostName a.example"}
	one := []RemoteDesktopEntry{{Name: "pc", Address: "10.0.0.9", Port: 3389, Protocol: "rdp"}}

	added := ReplaceRemoteDesktopsSection(host, one)
	want := []string{RemoteDesktopsBegin, "# stm:rd=pc address=10.0.0.9 port=3389 protocol=rdp", RemoteDesktopsEnd, "", "Host a", "    HostName a.example"}
	if !reflect.DeepEqual(added, want) {
		t.Fatalf("added = %q", added)
	}
	entries, _ := Parse(bufio.NewScanner(strings.NewReader(strings.Join(added, "\n"))))
	if len(entries) != 1 || entries[0].Alias != "a" {
		t.Fatalf("hosts after add = %+v", entries)
	}

	two := append(one, RemoteDesktopEntry{Name: "mac", Address: "10.0.0.8", Port: 5900, Protocol: "vnc"})
	updated := ReplaceRemoteDesktopsSection(added, two)
	if got := ParseRemoteDesktops(updated); len(got) != 2 || len(updated) != len(added)+1 {
		t.Fatalf("updated = %q", updated)
	}

	removed := ReplaceRemoteDesktopsSection(updated, nil)
	if !reflect.DeepEqual(removed, host) {
		t.Fatalf("removed = %q", removed)
	}
	if got := ReplaceRemoteDesktopsSection(nil, one); len(got) != 3 {
		t.Fatalf("empty file = %q", got)
	}

	// A section in the middle stays there.
	middle := []string{"Host a", "", RemoteDesktopsBegin, "# stm:rd=pc address=x protocol=rdp", RemoteDesktopsEnd, "", "Host b"}
	if got := ReplaceRemoteDesktopsSection(middle, nil); !reflect.DeepEqual(got, []string{"Host a", "", "Host b"}) {
		t.Fatalf("middle removed = %q", got)
	}
}

func TestParseFileRemoteDesktopsFollowsIncludes(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "config")
	extra := filepath.Join(dir, "extra.conf")
	if err := os.WriteFile(main, []byte("Include extra.conf\n"+configWithSection), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(extra, []byte("# stm:remote-desktops-begin\n# stm:rd=nas address=nas.lan protocol=vnc\n# stm:remote-desktops-end\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	desktops, err := ParseFileRemoteDesktops(main)
	if err != nil {
		t.Fatal(err)
	}
	if len(desktops) != 3 {
		t.Fatalf("desktops = %+v", desktops)
	}
	sources := map[string]string{}
	for _, d := range desktops {
		sources[d.Name] = d.SourceFile
	}
	if sources["laptop"] != main || sources["nas"] != extra {
		t.Fatalf("sources = %v", sources)
	}
	hosts, err := ParseFile(main)
	if err != nil || len(hosts) != 2 {
		t.Fatalf("hosts = %+v %v", hosts, err)
	}
}

func TestASectionWithoutEndMarkerNeverHidesHosts(t *testing.T) {
	config := "# stm:remote-desktops-begin\n# stm:rd=pc address=10.0.0.5 protocol=rdp\n\n# stm:group=Office\nHost a\n    HostName a.example\nHost b\n"
	entries, err := Parse(bufio.NewScanner(strings.NewReader(config)))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].HostName != "a.example" || entries[0].Group != "Office" {
		t.Fatalf("hosts = %+v", entries)
	}
	all := lines(config)
	if got := ParseRemoteDesktops(all); len(got) != 1 {
		t.Fatalf("desktops = %+v", got)
	}
	updated := UpdateRemoteDesktopLines(all, "mac", &RemoteDesktopEntry{Name: "mac", Address: "m", Port: 5900, Protocol: "vnc"})
	if hosts, _ := Parse(bufio.NewScanner(strings.NewReader(strings.Join(updated, "\n")))); len(hosts) != 2 || hosts[0].Group != "Office" {
		t.Fatalf("hosts lost: %q", updated)
	}
	if got := ParseRemoteDesktops(updated); len(got) != 2 || updated[3] != RemoteDesktopsEnd {
		t.Fatalf("updated = %q", updated)
	}
}

func TestUpdateRemoteDesktopLinesKeepsWhatItCannotRead(t *testing.T) {
	all := []string{
		RemoteDesktopsBegin,
		"# office machines",
		"# stm:rd=pc address=10.0.0.5 protocol=RDP future=\"x y\"",
		"# stm:rd=broken protocol=ssh",
		RemoteDesktopsEnd,
		"",
		"Host a",
	}
	if got := ParseRemoteDesktops(all); len(got) != 1 || got[0].Protocol != "rdp" || len(got[0].Extra) != 1 {
		t.Fatalf("parsed = %+v", got)
	}
	updated := UpdateRemoteDesktopLines(all, "pc", &RemoteDesktopEntry{Name: "pc", Address: "10.0.0.6", Port: 3389, Protocol: "rdp"})
	want := []string{
		RemoteDesktopsBegin,
		"# office machines",
		`# stm:rd=pc address=10.0.0.6 port=3389 protocol=rdp future="x y"`,
		"# stm:rd=broken protocol=ssh",
		RemoteDesktopsEnd,
		"",
		"Host a",
	}
	if !reflect.DeepEqual(updated, want) {
		t.Fatalf("updated = %q", updated)
	}
	// Removing the last readable entry keeps the section for the other lines.
	removed := UpdateRemoteDesktopLines(updated, "pc", nil)
	if len(removed) != 6 || removed[0] != RemoteDesktopsBegin {
		t.Fatalf("removed = %q", removed)
	}
	// A section with nothing left goes away.
	only := UpdateRemoteDesktopLines([]string{RemoteDesktopsBegin, "# stm:rd=pc address=a protocol=rdp", RemoteDesktopsEnd, "", "Host a"}, "pc", nil)
	if !reflect.DeepEqual(only, []string{"Host a"}) {
		t.Fatalf("only = %q", only)
	}
}
