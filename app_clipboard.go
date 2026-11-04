package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"

	"ssh-tunnel-manager/internal/clipboard"
)

// ClipboardContent is the system clipboard for a remote desktop session. PNG is
// base64 encoded, because bound methods carry JSON.
type ClipboardContent struct {
	Text string `json:"text"`
	PNG  string `json:"png,omitempty"`
}

// ReadClipboard returns the system clipboard. Reading a picture starts a helper
// process, so the frontend asks for it only when the session gets focus.
func (a *App) ReadClipboard(includeImage bool) (ClipboardContent, error) {
	var content ClipboardContent
	if a.wails != nil {
		if text, ok := a.wails.Clipboard.Text(); ok {
			content.Text = text
		}
	}
	if !includeImage {
		return content, nil
	}
	image, err := clipboard.ReadImage()
	if err != nil {
		if errors.Is(err, clipboard.ErrUnavailable) {
			return content, nil
		}
		slog.Debug("reading clipboard image failed", "error", err)
		return content, nil
	}
	if len(image) > 0 && len(image) <= clipboard.MaxImageSize {
		content.PNG = base64.StdEncoding.EncodeToString(image)
	}
	return content, nil
}

// WriteClipboardText puts text from a remote desktop on the system clipboard.
func (a *App) WriteClipboardText(text string) error {
	return a.copyToClipboard(text)
}

// WriteClipboardImage puts a PNG from a remote desktop on the system clipboard.
func (a *App) WriteClipboardImage(encoded string) error {
	image, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("decoding clipboard image: %w", err)
	}
	if !clipboard.IsPNG(image) || len(image) > clipboard.MaxImageSize {
		return errors.New("the clipboard image is not a PNG of an accepted size")
	}
	return clipboard.WriteImage(image)
}

// SaveRemoteFile writes a file copied on a remote desktop to a location the user
// picks. It returns an empty path when the user cancelled.
func (a *App) SaveRemoteFile(name, encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decoding %s: %w", name, err)
	}
	slog.Info("saving a file from a remote desktop", "name", name, "bytes", len(data))
	path, err := a.saveDialog("Save file from remote desktop", name)
	if err != nil || path == "" {
		return "", err
	}
	if err := writeFileAtomic(path, data); err != nil {
		return "", fmt.Errorf("saving %s: %w", name, err)
	}
	return path, nil
}
