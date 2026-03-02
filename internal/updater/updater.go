// Package updater checks GitHub Releases for newer versions and delegates
// installation to platform-specific code.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// UpdateInfo holds the details of an available update.
type UpdateInfo struct {
	LatestVersion string `json:"latestVersion"`
	ReleaseUrl    string `json:"releaseUrl"`
	AssetUrl      string `json:"assetUrl"`
	ReleaseNotes  string `json:"releaseNotes"`
}

// Check contacts GitHub Releases for the given repo and returns update info
// if a newer release exists. Returns nil, nil when currentVersion is "dev"
// or when already up to date.
func Check(ctx context.Context, currentVersion, repo string) (*UpdateInfo, error) {
	if currentVersion == "dev" {
		return nil, nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building update request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
		HtmlUrl string `json:"html_url"`
		Body    string `json:"body"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadUrl string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decoding release response: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	newer, err := isNewer(latestVersion, currentVersion)
	if err != nil {
		return nil, fmt.Errorf("comparing versions: %w", err)
	}
	if !newer {
		return nil, nil
	}

	var assetUrl string
	for _, a := range release.Assets {
		if a.Name == platformAsset {
			assetUrl = a.BrowserDownloadUrl
			break
		}
	}
	if assetUrl == "" {
		return nil, fmt.Errorf("no asset named %q in release %s", platformAsset, latestVersion)
	}

	return &UpdateInfo{
		LatestVersion: latestVersion,
		ReleaseUrl:    release.HtmlUrl,
		AssetUrl:      assetUrl,
		ReleaseNotes:  release.Body,
	}, nil
}

// isNewer returns true if candidate is strictly greater than baseline.
// Both strings must be "major.minor.patch" (pre-release suffixes after
// '-' or '+' are stripped).
func isNewer(candidate, baseline string) (bool, error) {
	cv, err := parseVersion(candidate)
	if err != nil {
		return false, fmt.Errorf("parsing candidate %q: %w", candidate, err)
	}
	bv, err := parseVersion(baseline)
	if err != nil {
		return false, fmt.Errorf("parsing baseline %q: %w", baseline, err)
	}
	for i := range cv {
		if cv[i] > bv[i] {
			return true, nil
		}
		if cv[i] < bv[i] {
			return false, nil
		}
	}
	return false, nil
}

func parseVersion(v string) ([3]int, error) {
	// Strip pre-release / build metadata
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return [3]int{}, fmt.Errorf("expected major.minor.patch, got %q", v)
	}
	var result [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, fmt.Errorf("non-numeric segment %q: %w", p, err)
		}
		result[i] = n
	}
	return result, nil
}
