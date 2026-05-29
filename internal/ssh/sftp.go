package ssh

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"ssh-tunnel-manager/internal/config"
)

// FileEntry describes a single entry in a remote directory listing.
type FileEntry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	IsDir   bool      `json:"isDir"`
	IsLink  bool      `json:"isLink"`
	ModTime time.Time `json:"modTime"`
}

// SFTPSession represents an open SFTP session over an SSH connection.
type SFTPSession struct {
	SessionID string
	client    *ssh.Client
	sftpc     *sftp.Client
	home      string

	mu     sync.Mutex
	closed bool
}

// Home returns the remote home directory used as the default starting point.
func (s *SFTPSession) Home() string {
	return s.home
}

// List returns the entries in the given remote directory, sorted with
// directories first, then by name.
func (s *SFTPSession) List(remotePath string) ([]FileEntry, error) {
	if remotePath == "" {
		remotePath = s.home
	}
	cleaned := path.Clean(remotePath)
	infos, err := s.sftpc.ReadDir(cleaned)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", cleaned, err)
	}
	entries := make([]FileEntry, 0, len(infos))
	for _, fi := range infos {
		entries = append(entries, FileEntry{
			Name:    fi.Name(),
			Path:    path.Join(cleaned, fi.Name()),
			Size:    fi.Size(),
			Mode:    fi.Mode().String(),
			IsDir:   fi.IsDir(),
			IsLink:  fi.Mode()&os.ModeSymlink != 0,
			ModTime: fi.ModTime(),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

// ProgressFunc is called periodically during a transfer with the number of
// bytes copied so far and the total size (when known; 0 if unknown).
type ProgressFunc func(transferred, total int64)

// Download copies the remote file to the local filesystem, reporting progress
// through onProgress if non-nil. Cancellation via ctx aborts the copy and
// removes the partial local file.
func (s *SFTPSession) Download(ctx context.Context, remotePath, localPath string, onProgress ProgressFunc) error {
	srcFile, err := s.sftpc.Open(remotePath)
	if err != nil {
		return fmt.Errorf("opening remote %s: %w", remotePath, err)
	}
	defer srcFile.Close()

	var total int64
	if stat, err := s.sftpc.Stat(remotePath); err == nil {
		total = stat.Size()
	}

	dstFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("creating local %s: %w", localPath, err)
	}
	defer dstFile.Close()

	pw := newProgressWriter(dstFile, total, onProgress)
	if _, err := io.Copy(pw, &ctxReader{ctx: ctx, r: srcFile}); err != nil {
		if ctx.Err() != nil {
			_ = dstFile.Close()
			_ = os.Remove(localPath)
			return ctx.Err()
		}
		return fmt.Errorf("copying %s: %w", remotePath, err)
	}
	pw.flush()
	return nil
}

// Upload copies a local file to the given remote directory, preserving its
// base name. Returns the remote path of the uploaded file. Cancellation via
// ctx aborts the copy and removes the partial remote file.
func (s *SFTPSession) Upload(ctx context.Context, localPath, remoteDir string, onProgress ProgressFunc) (string, error) {
	srcFile, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("opening local %s: %w", localPath, err)
	}
	defer srcFile.Close()

	var total int64
	if stat, err := srcFile.Stat(); err == nil {
		total = stat.Size()
	}

	remotePath := path.Join(remoteDir, filepath.Base(localPath))
	dstFile, err := s.sftpc.Create(remotePath)
	if err != nil {
		return "", fmt.Errorf("creating remote %s: %w", remotePath, err)
	}
	defer dstFile.Close()

	pw := newProgressWriter(dstFile, total, onProgress)
	if _, err := io.Copy(pw, &ctxReader{ctx: ctx, r: srcFile}); err != nil {
		if ctx.Err() != nil {
			_ = dstFile.Close()
			_ = s.sftpc.Remove(remotePath)
			return "", ctx.Err()
		}
		return "", fmt.Errorf("copying to %s: %w", remotePath, err)
	}
	pw.flush()
	return remotePath, nil
}

// DownloadDirZip walks the remote directory tree, archives every file into a
// ZIP at localPath, and reports progress as cumulative bytes written across
// the whole archive. Cancellation deletes the partial ZIP.
func (s *SFTPSession) DownloadDirZip(ctx context.Context, remoteRoot, localPath string, onProgress ProgressFunc) error {
	files, totalSize, err := s.walkFiles(remoteRoot)
	if err != nil {
		return fmt.Errorf("walking %s: %w", remoteRoot, err)
	}

	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", localPath, err)
	}
	zw := zip.NewWriter(out)

	rootBase := path.Base(remoteRoot)
	if rootBase == "" || rootBase == "/" {
		rootBase = "root"
	}

	if onProgress != nil {
		onProgress(0, totalSize)
	}

	var written int64
	lastEmit := time.Now()

	cleanup := func() {
		_ = zw.Close()
		_ = out.Close()
		_ = os.Remove(localPath)
	}

	for _, fi := range files {
		if err := ctx.Err(); err != nil {
			cleanup()
			return err
		}

		rel := strings.TrimPrefix(fi.path, remoteRoot)
		rel = strings.TrimPrefix(rel, "/")
		zipPath := rootBase
		if rel != "" {
			zipPath = rootBase + "/" + rel
		}

		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:     zipPath,
			Method:   zip.Deflate,
			Modified: fi.modTime,
		})
		if err != nil {
			cleanup()
			return fmt.Errorf("creating zip entry %s: %w", zipPath, err)
		}

		src, err := s.sftpc.Open(fi.path)
		if err != nil {
			cleanup()
			return fmt.Errorf("opening remote %s: %w", fi.path, err)
		}

		buf := make([]byte, 32*1024)
		for {
			if err := ctx.Err(); err != nil {
				src.Close()
				cleanup()
				return err
			}
			n, rerr := src.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					src.Close()
					cleanup()
					return fmt.Errorf("writing zip entry %s: %w", zipPath, werr)
				}
				written += int64(n)
				if onProgress != nil {
					now := time.Now()
					if now.Sub(lastEmit) >= 100*time.Millisecond {
						onProgress(written, totalSize)
						lastEmit = now
					}
				}
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				src.Close()
				cleanup()
				return fmt.Errorf("reading remote %s: %w", fi.path, rerr)
			}
		}
		src.Close()
	}

	if err := zw.Close(); err != nil {
		_ = out.Close()
		_ = os.Remove(localPath)
		return fmt.Errorf("closing zip: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(localPath)
		return fmt.Errorf("closing %s: %w", localPath, err)
	}
	if onProgress != nil {
		onProgress(written, totalSize)
	}
	return nil
}

type remoteFileInfo struct {
	path    string
	size    int64
	modTime time.Time
}

// walkFiles returns every regular file under root along with the cumulative
// byte total — used to drive zip download progress.
func (s *SFTPSession) walkFiles(root string) ([]remoteFileInfo, int64, error) {
	var out []remoteFileInfo
	var total int64
	walker := s.sftpc.Walk(root)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			return nil, 0, err
		}
		fi := walker.Stat()
		if fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
			continue
		}
		out = append(out, remoteFileInfo{
			path:    walker.Path(),
			size:    fi.Size(),
			modTime: fi.ModTime(),
		})
		total += fi.Size()
	}
	return out, total, nil
}

// ctxReader wraps an io.Reader and returns ctx.Err() once the context is
// cancelled, so io.Copy can be interrupted between reads.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// progressWriter wraps an io.Writer and emits throttled progress callbacks.
type progressWriter struct {
	w         io.Writer
	total     int64
	onProg    ProgressFunc
	written   int64
	lastEmit  time.Time
	lastBytes int64
}

func newProgressWriter(w io.Writer, total int64, onProg ProgressFunc) *progressWriter {
	pw := &progressWriter{w: w, total: total, onProg: onProg, lastEmit: time.Now()}
	if onProg != nil {
		onProg(0, total)
	}
	return pw
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.written += int64(n)
	if p.onProg != nil {
		now := time.Now()
		if now.Sub(p.lastEmit) >= 100*time.Millisecond && p.written != p.lastBytes {
			p.onProg(p.written, p.total)
			p.lastEmit = now
			p.lastBytes = p.written
		}
	}
	return n, err
}

func (p *progressWriter) flush() {
	if p.onProg != nil && p.written != p.lastBytes {
		p.onProg(p.written, p.total)
	}
}

// StatRemote returns FileInfo for the named entry inside remoteDir, or an
// error if it doesn't exist. Used to detect upload name conflicts.
func (s *SFTPSession) StatRemote(remoteDir, name string) (os.FileInfo, error) {
	return s.sftpc.Stat(path.Join(remoteDir, name))
}

const (
	// maxEditableSize is the soft cap above which a file is not opened for
	// editing unless the user forces it.
	maxEditableSize = 5 << 20 // 5 MiB
	// hardEditableSize is the absolute cap; files above this are never loaded
	// into the editor regardless of force.
	hardEditableSize = 50 << 20 // 50 MiB
	// binarySniffBytes is how many leading bytes are scanned for NUL bytes to
	// decide whether a file looks binary.
	binarySniffBytes = 8192
)

// TextFileResult carries the contents (and metadata) of a remote file opened
// for in-app editing. When the file is refused without force, Content is empty
// and either Binary or TooLarge is set so the UI can offer to open it anyway.
type TextFileResult struct {
	Content  string    `json:"content"`
	ModTime  time.Time `json:"modTime"`
	Size     int64     `json:"size"`
	Mode     string    `json:"mode"`
	Binary   bool      `json:"binary"`
	TooLarge bool      `json:"tooLarge"`
}

// ReadText loads a remote file as UTF-8 text for editing. When force is false
// the file is not read if it looks binary or exceeds maxEditableSize; the
// returned flags say which. Files above hardEditableSize are always refused.
func (s *SFTPSession) ReadText(remotePath string, force bool) (*TextFileResult, error) {
	st, err := s.sftpc.Stat(remotePath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", remotePath, err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("%s is a directory", remotePath)
	}
	res := &TextFileResult{
		ModTime: st.ModTime(),
		Size:    st.Size(),
		Mode:    st.Mode().String(),
	}
	if st.Size() > hardEditableSize {
		return nil, fmt.Errorf("%s is %d bytes, too large to edit", remotePath, st.Size())
	}
	if !force && st.Size() > maxEditableSize {
		res.TooLarge = true
		return res, nil
	}

	f, err := s.sftpc.Open(remotePath)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", remotePath, err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", remotePath, err)
	}
	if !force && looksBinary(data) {
		res.Binary = true
		return res, nil
	}
	res.Content = string(data)
	return res, nil
}

// looksBinary reports whether the leading bytes contain a NUL, the usual
// heuristic for distinguishing binary files from text.
func looksBinary(data []byte) bool {
	n := len(data)
	if n > binarySniffBytes {
		n = binarySniffBytes
	}
	return bytes.IndexByte(data[:n], 0) >= 0
}

// WriteText saves edited content back to remotePath. When expectModTime is
// non-zero and the remote file's modification time no longer matches (compared
// at second granularity), no write happens and changed is true so the caller
// can warn about a concurrent modification. The content is written to a temp
// sibling file and renamed into place so a failed write can't truncate the
// original, and the original file's mode is preserved. Returns the file's new
// modification time on success.
func (s *SFTPSession) WriteText(remotePath, content string, expectModTime time.Time) (changed bool, modTime time.Time, err error) {
	mode := os.FileMode(0o644)
	if st, statErr := s.sftpc.Stat(remotePath); statErr == nil {
		mode = st.Mode().Perm()
		if !expectModTime.IsZero() && st.ModTime().Unix() != expectModTime.Unix() {
			return true, st.ModTime(), nil
		}
	}

	tmpPath := fmt.Sprintf("%s.stm-%d.tmp", remotePath, time.Now().UnixNano())
	f, err := s.sftpc.Create(tmpPath)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("creating temp %s: %w", tmpPath, err)
	}
	if _, err := f.Write([]byte(content)); err != nil {
		_ = f.Close()
		_ = s.sftpc.Remove(tmpPath)
		return false, time.Time{}, fmt.Errorf("writing %s: %w", tmpPath, err)
	}
	if err := f.Close(); err != nil {
		_ = s.sftpc.Remove(tmpPath)
		return false, time.Time{}, fmt.Errorf("closing %s: %w", tmpPath, err)
	}
	_ = s.sftpc.Chmod(tmpPath, mode)

	// Prefer the atomic POSIX rename (replaces the target in one step); fall
	// back to remove + rename for servers without the OpenSSH extension.
	if err := s.sftpc.PosixRename(tmpPath, remotePath); err != nil {
		_ = s.sftpc.Remove(remotePath)
		if err2 := s.sftpc.Rename(tmpPath, remotePath); err2 != nil {
			_ = s.sftpc.Remove(tmpPath)
			return false, time.Time{}, fmt.Errorf("renaming %s -> %s: %w", tmpPath, remotePath, err2)
		}
	}

	var newMod time.Time
	if st, statErr := s.sftpc.Stat(remotePath); statErr == nil {
		newMod = st.ModTime()
	}
	return false, newMod, nil
}

// Mkdir creates a new directory at the given remote path.
func (s *SFTPSession) Mkdir(remotePath string) error {
	if err := s.sftpc.Mkdir(remotePath); err != nil {
		return fmt.Errorf("mkdir %s: %w", remotePath, err)
	}
	return nil
}

// Remove deletes the file or empty directory at the given remote path.
// For directories, it recurses through children first.
func (s *SFTPSession) Remove(remotePath string) error {
	info, err := s.sftpc.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("stat %s: %w", remotePath, err)
	}
	if info.IsDir() {
		return s.removeAll(remotePath)
	}
	if err := s.sftpc.Remove(remotePath); err != nil {
		return fmt.Errorf("removing %s: %w", remotePath, err)
	}
	return nil
}

func (s *SFTPSession) removeAll(dir string) error {
	entries, err := s.sftpc.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", dir, err)
	}
	for _, fi := range entries {
		full := path.Join(dir, fi.Name())
		if fi.IsDir() {
			if err := s.removeAll(full); err != nil {
				return err
			}
		} else {
			if err := s.sftpc.Remove(full); err != nil {
				return fmt.Errorf("removing %s: %w", full, err)
			}
		}
	}
	if err := s.sftpc.RemoveDirectory(dir); err != nil {
		return fmt.Errorf("removing directory %s: %w", dir, err)
	}
	return nil
}

// Rename renames or moves the entry at oldPath to newPath.
func (s *SFTPSession) Rename(oldPath, newPath string) error {
	if err := s.sftpc.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("renaming %s -> %s: %w", oldPath, newPath, err)
	}
	return nil
}

// Close tears down the SFTP and SSH connection.
func (s *SFTPSession) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	if s.sftpc != nil {
		_ = s.sftpc.Close()
	}
	if s.client != nil {
		_ = s.client.Close()
	}
}

// SFTPManager manages active SFTP sessions, keyed by session ID.
type SFTPManager struct {
	mu            sync.Mutex
	sessions      map[string]*SFTPSession
	nextID        int
	getPassphrase PassphraseFunc
}

// NewSFTPManager creates a new SFTPManager.
func NewSFTPManager(getPassphrase PassphraseFunc) *SFTPManager {
	return &SFTPManager{
		sessions:      make(map[string]*SFTPSession),
		getPassphrase: getPassphrase,
	}
}

// OpenSession establishes a new SSH+SFTP connection for the given tunnel
// configuration. Returns the session ID and the remote home directory.
func (sm *SFTPManager) OpenSession(cfg config.TunnelConfig) (string, string, error) {
	sshConfig, err := buildClientConfig(cfg, sm.getPassphrase)
	if err != nil {
		return "", "", fmt.Errorf("building SSH config for SFTP: %w", err)
	}

	tun := &Tunnel{Config: cfg, GetPassphrase: sm.getPassphrase}
	client, err := tun.dial(context.Background(), sshConfig)
	if err != nil {
		return "", "", fmt.Errorf("connecting SFTP to %s: %w", cfg.Host, err)
	}

	sftpc, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		return "", "", fmt.Errorf("opening SFTP subsystem: %w", err)
	}

	home, err := sftpc.Getwd()
	if err != nil || home == "" {
		home = "/"
	}

	sm.mu.Lock()
	sm.nextID++
	sessionID := fmt.Sprintf("sftp-%d", sm.nextID)
	s := &SFTPSession{
		SessionID: sessionID,
		client:    client,
		sftpc:     sftpc,
		home:      home,
	}
	sm.sessions[sessionID] = s
	sm.mu.Unlock()

	slog.Info("SFTP session opened", "sessionId", sessionID, "host", cfg.Host, "home", home)
	return sessionID, home, nil
}

// GetSession returns a session by ID.
func (sm *SFTPManager) GetSession(sessionID string) (*SFTPSession, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s, ok := sm.sessions[sessionID]
	return s, ok
}

// CloseSession closes and removes a session.
func (sm *SFTPManager) CloseSession(sessionID string) {
	sm.mu.Lock()
	s, ok := sm.sessions[sessionID]
	if ok {
		delete(sm.sessions, sessionID)
	}
	sm.mu.Unlock()
	if ok {
		s.Close()
		slog.Info("SFTP session closed", "sessionId", sessionID)
	}
}

// CloseAll closes every active SFTP session.
func (sm *SFTPManager) CloseAll() {
	sm.mu.Lock()
	sessions := make([]*SFTPSession, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		sessions = append(sessions, s)
	}
	sm.sessions = make(map[string]*SFTPSession)
	sm.mu.Unlock()
	for _, s := range sessions {
		s.Close()
	}
}
