---
globs: "**/*.go"
---

# Go Conventions

- Use `log/slog` for structured logging, never `fmt.Println` for logs
- Wrap errors with context: `fmt.Errorf("connecting to %s: %w", host, err)`
- Use `context.Context` for cancellation in all SSH operations
- Platform-specific code uses build tags: `//go:build darwin`, `//go:build windows`, `//go:build linux`
- Exported functions must have doc comments
- Use `sync.Mutex` for shared state in tunnel manager
- No global mutable state — pass dependencies via struct fields
- No CGo unless absolutely necessary
- Test files: `*_test.go` in the same package