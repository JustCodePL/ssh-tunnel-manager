package sshconfig

import (
	"fmt"
	"strconv"
	"strings"
)

// Markers of the section that holds remote desktops reached without SSH. The
// section is made of comments only, so ssh never sees its entries.
const (
	RemoteDesktopsBegin = "# stm:remote-desktops-begin"
	RemoteDesktopsEnd   = "# stm:remote-desktops-end"
	remoteDesktopPrefix = "# stm:rd="
)

// RemoteDesktopEntry is one remote desktop of the section: a machine reached
// directly over RDP or VNC, with no SSH server in between.
type RemoteDesktopEntry struct {
	Name       string
	Address    string
	Port       int
	Protocol   string
	Group      string
	Color      string
	Pinned     bool
	SourceFile string // absolute path to the file this entry was parsed from
	// Extra keeps keys this version does not know, as written, so a later
	// version's settings survive an edit here.
	Extra []string
}

func isSectionBegin(trimmed string) bool { return trimmed == RemoteDesktopsBegin }
func isSectionEnd(trimmed string) bool   { return trimmed == RemoteDesktopsEnd }

// isSectionMarker reports either marker of the remote desktops section.
func isSectionMarker(trimmed string) bool {
	return isSectionBegin(trimmed) || isSectionEnd(trimmed)
}

// inSectionLine reports a line that can belong to the section: an entry, a
// blank line or a plain comment. Anything else, another "# stm:" line such as
// the metadata of the next host included, ends a section whose end marker is
// missing, so a lost marker never hides or takes over the hosts below it.
func inSectionLine(trimmed string) bool {
	if trimmed == "" || strings.HasPrefix(trimmed, remoteDesktopPrefix) {
		return true
	}
	return strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "# stm:")
}

// sectionBounds returns the index of the begin marker and one past the end
// marker. A section without an end marker stops at its last comment before
// the first other line. start is -1 when the lines have no section.
func sectionBounds(lines []string) (start, end int) {
	start = -1
	last := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if start < 0 {
			if isSectionBegin(trimmed) {
				start, last = i, i
			}
			continue
		}
		if isSectionEnd(trimmed) {
			return start, i + 1
		}
		if !inSectionLine(trimmed) {
			break
		}
		if trimmed != "" {
			last = i
		}
	}
	if start < 0 {
		return -1, -1
	}
	return start, last + 1
}

// ParseRemoteDesktops returns the entries of the first remote desktops section
// in the lines. Lines that do not parse are skipped.
func ParseRemoteDesktops(lines []string) []RemoteDesktopEntry {
	start, end := sectionBounds(lines)
	if start < 0 {
		return nil
	}
	var entries []RemoteDesktopEntry
	for _, line := range lines[start+1 : end] {
		if entry, ok := ParseRemoteDesktopLine(strings.TrimSpace(line)); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

// ParseRemoteDesktopLine reads one "# stm:rd=<name> key=value ..." line.
// Values with spaces are quoted, with \" and \\ inside the quotes.
func ParseRemoteDesktopLine(line string) (RemoteDesktopEntry, bool) {
	if !strings.HasPrefix(line, remoteDesktopPrefix) {
		return RemoteDesktopEntry{}, false
	}
	fields, ok := splitFields(strings.TrimPrefix(line, "# stm:"))
	if !ok || len(fields) == 0 || fields[0].key != "rd" || fields[0].value == "" {
		return RemoteDesktopEntry{}, false
	}
	entry := RemoteDesktopEntry{Name: fields[0].value}
	for _, f := range fields[1:] {
		switch f.key {
		case "address":
			entry.Address = f.value
		case "port":
			if n, err := strconv.Atoi(f.value); err == nil && n >= 1 && n <= 65535 {
				entry.Port = n
			}
		case "protocol":
			entry.Protocol = f.value
		case "group":
			entry.Group = f.value
		case "color":
			entry.Color = f.value
		case "pinned":
			entry.Pinned = f.value == "true"
		default:
			entry.Extra = append(entry.Extra, f.key+"="+quoteValue(f.value))
		}
	}
	entry.Protocol = strings.ToLower(entry.Protocol)
	if entry.Address == "" || (entry.Protocol != "rdp" && entry.Protocol != "vnc") {
		return RemoteDesktopEntry{}, false
	}
	if entry.Port == 0 {
		entry.Port = DefaultRemoteDesktopPort(entry.Protocol)
	}
	return entry, true
}

// DefaultRemoteDesktopPort is the standard port of a remote desktop protocol.
func DefaultRemoteDesktopPort(protocol string) int {
	if protocol == "vnc" {
		return 5900
	}
	return 3389
}

type field struct{ key, value string }

// splitFields splits `a=1 b="two words"` into key and value pairs.
func splitFields(s string) ([]field, bool) {
	var fields []field
	i := 0
	for {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			return fields, true
		}
		eq := strings.IndexByte(s[i:], '=')
		if eq <= 0 {
			return nil, false
		}
		key := s[i : i+eq]
		if strings.ContainsAny(key, " \t\"") {
			return nil, false
		}
		i += eq + 1
		var value strings.Builder
		if i < len(s) && s[i] == '"' {
			i++
			closed := false
			for i < len(s) {
				c := s[i]
				if c == '\\' && i+1 < len(s) {
					value.WriteByte(s[i+1])
					i += 2
					continue
				}
				if c == '"' {
					closed = true
					i++
					break
				}
				value.WriteByte(c)
				i++
			}
			if !closed {
				return nil, false
			}
		} else {
			for i < len(s) && s[i] != ' ' && s[i] != '\t' {
				value.WriteByte(s[i])
				i++
			}
		}
		fields = append(fields, field{key: key, value: value.String()})
	}
}

// quoteValue quotes a value that would not survive splitFields as it is.
func quoteValue(v string) string {
	if v != "" && !strings.ContainsAny(v, " \t\"\\") {
		return v
	}
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	return `"` + v + `"`
}

// RenderRemoteDesktopLine writes one entry of the section.
func RenderRemoteDesktopLine(e RemoteDesktopEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s address=%s port=%d protocol=%s", remoteDesktopPrefix, quoteValue(e.Name), quoteValue(e.Address), e.Port, e.Protocol)
	if e.Group != "" {
		fmt.Fprintf(&b, " group=%s", quoteValue(e.Group))
	}
	if e.Color != "" {
		fmt.Fprintf(&b, " color=%s", quoteValue(e.Color))
	}
	if e.Pinned {
		b.WriteString(" pinned=true")
	}
	for _, extra := range e.Extra {
		b.WriteString(" " + extra)
	}
	return b.String()
}

// RenderRemoteDesktopsSection writes the whole section, markers included.
func RenderRemoteDesktopsSection(entries []RemoteDesktopEntry) []string {
	lines := []string{RemoteDesktopsBegin}
	for _, e := range entries {
		lines = append(lines, RenderRemoteDesktopLine(e))
	}
	return append(lines, RemoteDesktopsEnd)
}

// ReplaceRemoteDesktopsSection writes entries as the file's section. An
// existing section is replaced where it is; a new one goes to the top of the
// file, followed by a blank line, where versions that do not know the section
// cannot take it for part of a Host block. No entries removes the section.
func ReplaceRemoteDesktopsSection(lines []string, entries []RemoteDesktopEntry) []string {
	start, end := sectionBounds(lines)
	if start >= 0 {
		var section []string
		if len(entries) > 0 {
			section = RenderRemoteDesktopsSection(entries)
		}
		result := make([]string, 0, len(lines)-(end-start)+len(section))
		result = append(result, lines[:start]...)
		result = append(result, section...)
		result = append(result, lines[end:]...)
		if len(entries) == 0 {
			result = cleanBlankLines(result, start)
			// A section at the top leaves its separator behind.
			for len(result) > 0 && start == 0 && strings.TrimSpace(result[0]) == "" {
				result = result[1:]
			}
		}
		return result
	}
	if len(entries) == 0 {
		return lines
	}
	section := RenderRemoteDesktopsSection(entries)
	result := make([]string, 0, len(lines)+len(section)+1)
	result = append(result, section...)
	if len(lines) > 0 {
		if strings.TrimSpace(lines[0]) != "" {
			result = append(result, "")
		}
		result = append(result, lines...)
	}
	return result
}

// UpdateRemoteDesktopLines changes one entry of the file's section and keeps
// every other line as it is, including comments and entries this version
// cannot read. It replaces the entry called name with e, adds e when there is
// no such entry, or removes the entry when e is nil. Keys of the old line this
// version does not know are kept. A section left with nothing in it is removed.
func UpdateRemoteDesktopLines(lines []string, name string, e *RemoteDesktopEntry) []string {
	start, end := sectionBounds(lines)
	if start < 0 {
		if e == nil {
			return lines
		}
		return ReplaceRemoteDesktopsSection(lines, []RemoteDesktopEntry{*e})
	}
	body := make([]string, 0, end-start)
	for _, line := range lines[start+1 : end] {
		if !isSectionEnd(strings.TrimSpace(line)) {
			body = append(body, line)
		}
	}
	found := false
	var kept []string
	for _, line := range body {
		old, ok := ParseRemoteDesktopLine(strings.TrimSpace(line))
		if !ok || old.Name != name || found {
			kept = append(kept, line)
			continue
		}
		found = true
		if e != nil {
			updated := *e
			updated.Extra = old.Extra
			kept = append(kept, RenderRemoteDesktopLine(updated))
		}
	}
	if !found && e != nil {
		kept = append(kept, RenderRemoteDesktopLine(*e))
	}
	empty := true
	for _, line := range kept {
		if strings.TrimSpace(line) != "" {
			empty = false
		}
	}
	var section []string
	if !empty {
		section = append(append([]string{RemoteDesktopsBegin}, kept...), RemoteDesktopsEnd)
	}
	result := make([]string, 0, len(lines)-(end-start)+len(section))
	result = append(result, lines[:start]...)
	result = append(result, section...)
	result = append(result, lines[end:]...)
	if empty {
		result = cleanBlankLines(result, start)
		for len(result) > 0 && start == 0 && strings.TrimSpace(result[0]) == "" {
			result = result[1:]
		}
	}
	return result
}

// ParseFileRemoteDesktops reads the remote desktops sections of a config file
// and of every file it includes. Each entry has SourceFile set.
func ParseFileRemoteDesktops(path string) ([]RemoteDesktopEntry, error) {
	files, err := IncludedConfigFiles(path)
	if err != nil {
		return nil, err
	}
	var all []RemoteDesktopEntry
	for _, file := range files {
		lines, err := ReadLines(file)
		if err != nil {
			return nil, err
		}
		for _, entry := range ParseRemoteDesktops(lines) {
			entry.SourceFile = file
			all = append(all, entry)
		}
	}
	return all, nil
}
