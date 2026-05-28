package sshconfig

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// PortForwardEntry defines a single local-to-remote port mapping parsed from
// a LocalForward directive. Portless/Domain/ExposePort/HostHeader are picked
// up from "# stm:forward-..." comments placed on the lines immediately above
// the LocalForward.
type PortForwardEntry struct {
	LocalPort     int
	RemoteHost    string
	RemotePort    int
	Description   string
	Portless      bool
	Domain        string
	ExposePort    int
	HostHeader    string
	HostHeaderOff bool
}

// HostEntry holds the parsed fields of a single Host block in an SSH config
// file, including app-specific stm metadata.
type HostEntry struct {
	Alias        string
	HostName     string
	Port         int
	User         string
	IdentityFile string
	PortForwards []PortForwardEntry
	ProxyCommand string
	ProxyJump    string
	Color        string
	Group        string
	AutoConnect  bool
	Pinned       bool
	SourceFile   string // absolute path to the file this entry was parsed from
}

// ParseFile reads an SSH config file and returns host entries for all
// non-wildcard, non-Match Host entries. Wildcard hosts (containing * or ?)
// and Match blocks are skipped. Returns nil, nil for a missing file.
// Include directives are expanded: globs are resolved and included files
// are parsed recursively. Each returned HostEntry has SourceFile set to
// the absolute path of the file it was parsed from.
func ParseFile(path string) ([]HostEntry, error) {
	return parseFileWithVisited(path, nil)
}

func parseFileWithVisited(path string, visited map[string]bool) ([]HostEntry, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving path %s: %w", path, err)
	}

	if visited == nil {
		visited = make(map[string]bool)
	}
	if visited[absPath] {
		return nil, nil // avoid circular includes
	}
	visited[absPath] = true

	f, err := os.Open(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("opening ssh config %s: %w", absPath, err)
	}
	defer f.Close()

	// First pass: collect Include directives and their line positions
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning ssh config %s: %w", absPath, err)
	}

	var allEntries []HostEntry
	var chunk []string
	baseDir := filepath.Dir(absPath)

	flushChunk := func() error {
		if len(chunk) == 0 {
			return nil
		}
		s := bufio.NewScanner(strings.NewReader(strings.Join(chunk, "\n")))
		entries, err := Parse(s)
		if err != nil {
			return err
		}
		for i := range entries {
			entries[i].SourceFile = absPath
		}
		allEntries = append(allEntries, entries...)
		chunk = nil
		return nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isIncludeDirective(trimmed) {
			// Flush any accumulated lines before the Include
			if err := flushChunk(); err != nil {
				return nil, err
			}
			pattern := extractIncludePattern(trimmed, baseDir)
			matches, err := filepath.Glob(pattern)
			if err != nil {
				return nil, fmt.Errorf("expanding Include glob %q: %w", pattern, err)
			}
			for _, m := range matches {
				included, err := parseFileWithVisited(m, visited)
				if err != nil {
					return nil, fmt.Errorf("parsing included file %s: %w", m, err)
				}
				allEntries = append(allEntries, included...)
			}
		} else {
			chunk = append(chunk, line)
		}
	}

	if err := flushChunk(); err != nil {
		return nil, err
	}

	return allEntries, nil
}

// Parse reads SSH config lines from a scanner and returns host entries.
func Parse(scanner *bufio.Scanner) ([]HostEntry, error) {
	var entries []HostEntry
	var current *HostEntry
	var metaGroup string
	var metaColor string
	var metaAutoConnect bool
	var metaPinned bool
	var pendingFwdPortless bool
	var pendingFwdDomain string
	var pendingFwdExposePort int
	var pendingFwdHostHeader string
	var pendingFwdHostHeaderOff bool
	inMatch := false

	resetPendingForward := func() {
		pendingFwdPortless = false
		pendingFwdDomain = ""
		pendingFwdExposePort = 0
		pendingFwdHostHeader = ""
		pendingFwdHostHeaderOff = false
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Collect stm metadata comments
		if strings.HasPrefix(trimmed, "# stm:") {
			key, val := parseMetaComment(trimmed)
			switch key {
			case "group":
				metaGroup = val
			case "color":
				metaColor = val
			case "autoconnect":
				metaAutoConnect = val == "true"
			case "pinned":
				metaPinned = val == "true"
			case "forward-portless":
				pendingFwdPortless = val == "true"
			case "forward-domain":
				pendingFwdDomain = val
			case "forward-expose-port", "forward-local-port":
				// forward-local-port is the legacy key from an earlier
				// iteration of this feature; accepted for backward compat.
				if n, err := strconv.Atoi(val); err == nil {
					pendingFwdExposePort = n
				}
			case "forward-host-header":
				pendingFwdHostHeader = val
			case "forward-host-header-off":
				pendingFwdHostHeaderOff = val == "true"
			}
			continue
		}

		// Match block — skip until next Host or Match
		if isMatchDirective(trimmed) {
			if current != nil {
				entries = appendIfValid(entries, current)
				current = nil
			}
			inMatch = true
			metaGroup = ""
			metaColor = ""
			metaAutoConnect = false
			metaPinned = false
			continue
		}

		// Host directive
		if isHostDirective(trimmed) {
			if current != nil {
				entries = appendIfValid(entries, current)
			}
			inMatch = false
			alias := extractHostAlias(trimmed)

			if isWildcard(alias) {
				current = nil
				metaGroup = ""
				metaColor = ""
				metaAutoConnect = false
				metaPinned = false
				resetPendingForward()
				continue
			}

			current = &HostEntry{
				Alias:       alias,
				Port:        22,
				Color:       metaColor,
				Group:       metaGroup,
				AutoConnect: metaAutoConnect,
				Pinned:      metaPinned,
			}
			metaGroup = ""
			metaColor = ""
			metaAutoConnect = false
			metaPinned = false
			resetPendingForward()
			continue
		}

		if inMatch || current == nil {
			// Reset pending metadata if we see a non-comment, non-Host line
			// while not in a Host block
			if current == nil && !strings.HasPrefix(trimmed, "#") && trimmed != "" {
				metaGroup = ""
				metaColor = ""
				metaAutoConnect = false
				metaPinned = false
			}
			continue
		}

		// Parse directives within a Host block
		key, val := parseDirective(trimmed)
		if key == "" {
			continue
		}

		lowered := strings.ToLower(key)
		// Any directive other than LocalForward consumes/drops the pending
		// per-forward metadata so it doesn't bleed into an unrelated forward.
		if lowered != "localforward" {
			resetPendingForward()
		}
		switch lowered {
		case "hostname":
			current.HostName = val
		case "port":
			if p, err := strconv.Atoi(val); err == nil {
				current.Port = p
			}
		case "user":
			current.User = val
		case "identityfile":
			current.IdentityFile = expandTilde(val)
		case "localforward":
			if pf, err := parseLocalForward(val); err == nil {
				pf.Portless = pendingFwdPortless
				pf.Domain = pendingFwdDomain
				pf.ExposePort = pendingFwdExposePort
				pf.HostHeader = pendingFwdHostHeader
				pf.HostHeaderOff = pendingFwdHostHeaderOff
				current.PortForwards = append(current.PortForwards, pf)
			}
			resetPendingForward()
		case "proxycommand":
			current.ProxyCommand = val
		case "proxyjump":
			current.ProxyJump = val
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning ssh config: %w", err)
	}

	if current != nil {
		entries = appendIfValid(entries, current)
	}

	// If HostName is absent, default to the alias
	for i := range entries {
		if entries[i].HostName == "" {
			entries[i].HostName = entries[i].Alias
		}
	}

	return entries, nil
}

// RenderHostBlock produces SSH config text for a single HostEntry,
// including stm metadata comments. The block is ready to append to a file.
func RenderHostBlock(e HostEntry) string {
	var b strings.Builder

	if e.Group != "" {
		fmt.Fprintf(&b, "# stm:group=%s\n", e.Group)
	}
	if e.Color != "" {
		fmt.Fprintf(&b, "# stm:color=%s\n", e.Color)
	}
	if e.AutoConnect {
		b.WriteString("# stm:autoconnect=true\n")
	}
	if e.Pinned {
		b.WriteString("# stm:pinned=true\n")
	}

	fmt.Fprintf(&b, "Host %s\n", e.Alias)

	if e.HostName != "" && e.HostName != e.Alias {
		fmt.Fprintf(&b, "    HostName %s\n", e.HostName)
	}
	if e.User != "" {
		fmt.Fprintf(&b, "    User %s\n", e.User)
	}
	if e.Port != 0 && e.Port != 22 {
		fmt.Fprintf(&b, "    Port %d\n", e.Port)
	}
	if e.IdentityFile != "" {
		fmt.Fprintf(&b, "    IdentityFile %s\n", collapseTilde(e.IdentityFile))
	}
	if e.ProxyCommand != "" {
		fmt.Fprintf(&b, "    ProxyCommand %s\n", e.ProxyCommand)
	}
	if e.ProxyJump != "" {
		fmt.Fprintf(&b, "    ProxyJump %s\n", e.ProxyJump)
	}
	for _, pf := range e.PortForwards {
		if pf.Portless {
			b.WriteString("    # stm:forward-portless=true\n")
		}
		if pf.Domain != "" {
			fmt.Fprintf(&b, "    # stm:forward-domain=%s\n", pf.Domain)
		}
		if pf.ExposePort > 0 {
			fmt.Fprintf(&b, "    # stm:forward-expose-port=%d\n", pf.ExposePort)
		}
		if pf.HostHeader != "" {
			fmt.Fprintf(&b, "    # stm:forward-host-header=%s\n", pf.HostHeader)
		}
		if pf.HostHeaderOff {
			b.WriteString("    # stm:forward-host-header-off=true\n")
		}
		// LocalForward needs a numeric port — use LocalPort if set, otherwise
		// fall back to RemotePort so the line stays valid for plain `ssh`
		// users invoking this host from a terminal.
		writtenLocalPort := pf.LocalPort
		if writtenLocalPort <= 0 {
			writtenLocalPort = pf.RemotePort
		}
		if pf.Description != "" {
			fmt.Fprintf(&b, "    LocalForward %d %s:%d # %s\n", writtenLocalPort, pf.RemoteHost, pf.RemotePort, pf.Description)
		} else {
			fmt.Fprintf(&b, "    LocalForward %d %s:%d\n", writtenLocalPort, pf.RemoteHost, pf.RemotePort)
		}
	}

	return b.String()
}

func appendIfValid(entries []HostEntry, e *HostEntry) []HostEntry {
	if e != nil && e.Alias != "" {
		return append(entries, *e)
	}
	return entries
}

func parseMetaComment(line string) (key, val string) {
	// "# stm:key=value"
	after := strings.TrimPrefix(line, "# stm:")
	parts := strings.SplitN(after, "=", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func isHostDirective(line string) bool {
	lower := strings.ToLower(line)
	return strings.HasPrefix(lower, "host ") || strings.HasPrefix(lower, "host\t")
}

func isMatchDirective(line string) bool {
	lower := strings.ToLower(line)
	return strings.HasPrefix(lower, "match ") || strings.HasPrefix(lower, "match\t")
}

func extractHostAlias(line string) string {
	// "Host alias" or "Host  alias"
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		return fields[1]
	}
	return ""
}

func isWildcard(alias string) bool {
	return strings.ContainsAny(alias, "*?")
}

func parseDirective(line string) (key, val string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", ""
	}

	// Handle "Key=Value" syntax
	if idx := strings.Index(trimmed, "="); idx > 0 {
		k := strings.TrimSpace(trimmed[:idx])
		v := strings.TrimSpace(trimmed[idx+1:])
		if !strings.ContainsAny(k, " \t") {
			return k, v
		}
	}

	// Handle "Key Value" syntax
	fields := strings.SplitN(trimmed, " ", 2)
	if len(fields) == 1 {
		fields = strings.SplitN(trimmed, "\t", 2)
	}
	if len(fields) != 2 {
		return "", ""
	}
	return strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1])
}

// parseLocalForward parses "localPort remoteHost:remotePort" with an optional
// inline comment used as description: "localPort remoteHost:remotePort # desc".
func parseLocalForward(val string) (PortForwardEntry, error) {
	// Extract inline comment as description
	var description string
	if idx := strings.Index(val, "#"); idx >= 0 {
		description = strings.TrimSpace(val[idx+1:])
		val = strings.TrimSpace(val[:idx])
	}

	fields := strings.Fields(val)
	if len(fields) < 2 {
		return PortForwardEntry{}, fmt.Errorf("invalid LocalForward: %q", val)
	}

	localPort, err := strconv.Atoi(fields[0])
	if err != nil {
		return PortForwardEntry{}, fmt.Errorf("invalid local port in LocalForward %q: %w", val, err)
	}

	remote := fields[1]
	lastColon := strings.LastIndex(remote, ":")
	if lastColon < 0 {
		return PortForwardEntry{}, fmt.Errorf("invalid remote in LocalForward %q: missing colon", val)
	}

	remoteHost := remote[:lastColon]
	remotePort, err := strconv.Atoi(remote[lastColon+1:])
	if err != nil {
		return PortForwardEntry{}, fmt.Errorf("invalid remote port in LocalForward %q: %w", val, err)
	}

	return PortForwardEntry{
		LocalPort:   localPort,
		RemoteHost:  remoteHost,
		RemotePort:  remotePort,
		Description: description,
	}, nil
}

func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func collapseTilde(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home+"/") {
		return "~/" + path[len(home)+1:]
	}
	return path
}

func isIncludeDirective(line string) bool {
	lower := strings.ToLower(line)
	return strings.HasPrefix(lower, "include ") || strings.HasPrefix(lower, "include\t")
}

func extractIncludePattern(line, baseDir string) string {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return ""
	}
	pattern := fields[1]
	pattern = expandTilde(pattern)
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(baseDir, pattern)
	}
	return pattern
}

// CollapseTildePath collapses an absolute path to use ~/ prefix if it's
// under the user's home directory. Exported for use by the frontend.
func CollapseTildePath(path string) string {
	return collapseTilde(path)
}

// IncludedConfigFiles returns the main config path followed by the files
// referenced by any Include directives found while scanning the given file.
func IncludedConfigFiles(path string) ([]string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving path %s: %w", path, err)
	}

	seen := make(map[string]bool)
	files := []string{}

	var collect func(string) error
	collect = func(p string) error {
		abs, err := filepath.Abs(p)
		if err != nil {
			return fmt.Errorf("resolving path %s: %w", p, err)
		}
		if seen[abs] {
			return nil
		}
		seen[abs] = true
		files = append(files, abs)

		f, err := os.Open(abs)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("opening ssh config %s: %w", abs, err)
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		baseDir := filepath.Dir(abs)
		for scanner.Scan() {
			trimmed := strings.TrimSpace(scanner.Text())
			if isIncludeDirective(trimmed) {
				pattern := extractIncludePattern(trimmed, baseDir)
				if pattern == "" {
					continue
				}
				matches, err := filepath.Glob(pattern)
				if err != nil {
					return fmt.Errorf("expanding Include glob %q: %w", pattern, err)
				}
				for _, match := range matches {
					if err := collect(match); err != nil {
						return err
					}
				}
			}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("scanning ssh config %s: %w", abs, err)
		}
		return nil
	}

	if err := collect(absPath); err != nil {
		return nil, err
	}
	return files, nil
}
