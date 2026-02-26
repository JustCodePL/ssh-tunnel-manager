package sshconfig

import (
	"fmt"
	"os"
	"strings"
)

// HostBlock represents a contiguous block of lines in the SSH config file
// belonging to a single Host entry, including any preceding stm metadata
// comments.
type HostBlock struct {
	// StartLine is the 0-based index of the first line (metadata comment or Host directive).
	StartLine int
	// EndLine is the 0-based index one past the last line of the block.
	EndLine int
	// Alias is the Host alias extracted from the Host directive.
	Alias string
}

// ReadLines reads a file into a slice of lines.
func ReadLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	content := string(data)
	// Preserve trailing newline behavior: if file ends with \n, don't create
	// an extra empty element.
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return nil, nil
	}
	return strings.Split(content, "\n"), nil
}

// WriteLines writes lines back to a file atomically (write tmp + rename).
// A trailing newline is appended.
func WriteLines(path string, lines []string) error {
	content := strings.Join(lines, "\n") + "\n"
	tmp := path + ".stm.tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing temp file %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("renaming %s -> %s: %w", tmp, path, err)
	}
	return nil
}

// FindHostBlock locates the Host block with the given alias in the lines.
// Returns nil if not found.
func FindHostBlock(lines []string, alias string) *HostBlock {
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if isHostDirective(trimmed) && extractHostAlias(trimmed) == alias {
			// Found the Host line. Scan backwards for stm metadata comments.
			start := i
			for start > 0 {
				prev := strings.TrimSpace(lines[start-1])
				if strings.HasPrefix(prev, "# stm:") {
					start--
				} else {
					break
				}
			}

			// Scan forward for the end of the block (next Host, Match, or EOF).
			end := i + 1
			for end < len(lines) {
				t := strings.TrimSpace(lines[end])
				if isHostDirective(t) || isMatchDirective(t) {
					break
				}
				// Also stop at stm metadata that precedes the next Host
				if strings.HasPrefix(t, "# stm:") && end+1 < len(lines) {
					nextNonMeta := end
					for nextNonMeta < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[nextNonMeta]), "# stm:") {
						nextNonMeta++
					}
					if nextNonMeta < len(lines) && isHostDirective(strings.TrimSpace(lines[nextNonMeta])) {
						break
					}
				}
				end++
			}

			return &HostBlock{StartLine: start, EndLine: end, Alias: alias}
		}
	}
	return nil
}

// RemoveHostBlock removes the lines belonging to the named Host block.
// Returns the modified lines, or an error if the host is not found.
func RemoveHostBlock(lines []string, alias string) ([]string, error) {
	block := FindHostBlock(lines, alias)
	if block == nil {
		return nil, fmt.Errorf("host %q not found", alias)
	}

	// Remove the block lines
	result := make([]string, 0, len(lines)-(block.EndLine-block.StartLine))
	result = append(result, lines[:block.StartLine]...)
	result = append(result, lines[block.EndLine:]...)

	// Clean up extra blank lines at the splice point
	result = cleanBlankLines(result, block.StartLine)

	return result, nil
}

// ReplaceHostBlock replaces the lines of the named Host block with new content.
// Returns the modified lines, or an error if the host is not found.
func ReplaceHostBlock(lines []string, alias string, newContent string) ([]string, error) {
	block := FindHostBlock(lines, alias)
	if block == nil {
		return nil, fmt.Errorf("host %q not found", alias)
	}

	newLines := splitContent(newContent)

	result := make([]string, 0, len(lines)-(block.EndLine-block.StartLine)+len(newLines))
	result = append(result, lines[:block.StartLine]...)
	result = append(result, newLines...)
	result = append(result, lines[block.EndLine:]...)

	return result, nil
}

// AppendHostBlock appends a new Host block to the end of the lines.
// Ensures a blank line separator before the new block.
func AppendHostBlock(lines []string, content string) []string {
	newLines := splitContent(content)

	// Ensure blank line separator if file is non-empty
	if len(lines) > 0 {
		last := lines[len(lines)-1]
		if strings.TrimSpace(last) != "" {
			lines = append(lines, "")
		}
	}

	return append(lines, newLines...)
}

func splitContent(content string) []string {
	// Remove trailing newline to avoid empty last element
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

// cleanBlankLines removes excess blank lines at the given position.
// Keeps at most one blank line between blocks.
func cleanBlankLines(lines []string, pos int) []string {
	// Count blank lines at pos
	blanks := 0
	for i := pos; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			blanks++
		} else {
			break
		}
	}

	// Also count blank lines just before pos
	blanksBefore := 0
	for i := pos - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			blanksBefore++
		} else {
			break
		}
	}

	totalBlanks := blanksBefore + blanks
	if totalBlanks <= 1 {
		return lines
	}

	// Remove excess: keep at most 1 blank line total in this gap
	removeStart := pos - blanksBefore
	removeEnd := pos + blanks
	keep := 0
	// If there's content both before and after, keep one blank line
	if removeStart > 0 && removeEnd < len(lines) {
		keep = 1
	}

	toRemove := totalBlanks - keep
	if toRemove <= 0 {
		return lines
	}

	result := make([]string, 0, len(lines)-toRemove)
	result = append(result, lines[:removeStart+keep]...)
	result = append(result, lines[removeEnd:]...)
	return result
}
