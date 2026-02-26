Review the current codebase for:

1. **Correctness** — bugs, race conditions, unhandled errors?
2. **SSH safety** — connections properly closed? goroutine leaks? context cancellation?
3. **Cross-platform** — works on Windows/macOS/Linux? missing build tags?
4. **Security** — secrets in config? unsafe operations?
5. **UX** — error messages clear and actionable?

Focus on `internal/` Go code first, then frontend.
Output a prioritized list of issues with code-level fixes.