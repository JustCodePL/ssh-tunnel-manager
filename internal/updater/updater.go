// Package updater checks GitHub Releases for newer versions and delegates
// installation to platform-specific code.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// Channel controls which GitHub releases are eligible for updates.
type Channel string

const (
	ChannelStable Channel = "stable"
	ChannelBeta   Channel = "beta"
)

// ParseChannel validates a persisted or user-supplied update channel.
func ParseChannel(value string) (Channel, error) {
	channel := Channel(value)
	switch channel {
	case ChannelStable, ChannelBeta:
		return channel, nil
	default:
		return "", fmt.Errorf("invalid update channel %q", value)
	}
}

// UpdateInfo holds the details of an available update.
type UpdateInfo struct {
	LatestVersion string `json:"latestVersion"`
	ReleaseUrl    string `json:"releaseUrl"`
	AssetUrl      string `json:"assetUrl"`
	ReleaseNotes  string `json:"releaseNotes"`
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	HtmlURL    string `json:"html_url"`
	Body       string `json:"body"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// Check contacts GitHub Releases and returns the newest release allowed by
// channel. Stable accepts full releases only; beta accepts both prereleases
// and full releases so beta users naturally advance to the final release.
// Switching a prerelease build to Stable intentionally allows the newest
// stable release even when its version is lower than the current beta.
// Returns nil, nil when currentVersion is "dev" or when already up to date.
func Check(ctx context.Context, currentVersion, repo string, channel Channel) (*UpdateInfo, error) {
	return check(ctx, currentVersion, repo, channel, "https://api.github.com", http.DefaultClient)
}

func check(ctx context.Context, currentVersion, repo string, channel Channel, apiBaseURL string, client *http.Client) (*UpdateInfo, error) {
	if currentVersion == "dev" {
		return nil, nil
	}
	if _, err := ParseChannel(string(channel)); err != nil {
		return nil, err
	}

	current, err := normalizeVersion(currentVersion)
	if err != nil {
		return nil, fmt.Errorf("parsing current version %q: %w", currentVersion, err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	url := strings.TrimRight(apiBaseURL, "/") + "/repos/" + repo + "/releases?per_page=100"
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building update request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decoding releases response: %w", err)
	}

	var selected *githubRelease
	selectedVersion := ""
	switchingPrereleaseToStable := channel == ChannelStable && semver.Prerelease(current) != ""
	for i := range releases {
		release := &releases[i]
		if release.Draft || (channel == ChannelStable && release.Prerelease) {
			continue
		}

		candidate, err := normalizeVersion(release.TagName)
		if err != nil {
			continue
		}
		if !switchingPrereleaseToStable && semver.Compare(candidate, current) <= 0 {
			continue
		}
		if selected == nil || semver.Compare(candidate, selectedVersion) > 0 {
			selected = release
			selectedVersion = candidate
		}
	}

	if selected == nil {
		return nil, nil
	}

	assetURL := ""
	for _, asset := range selected.Assets {
		if asset.Name == platformAsset {
			assetURL = asset.BrowserDownloadURL
			break
		}
	}
	if assetURL == "" {
		return nil, fmt.Errorf("no asset named %q in release %s", platformAsset, strings.TrimPrefix(selectedVersion, "v"))
	}

	return &UpdateInfo{
		LatestVersion: strings.TrimPrefix(selectedVersion, "v"),
		ReleaseUrl:    selected.HtmlURL,
		AssetUrl:      assetURL,
		ReleaseNotes:  selected.Body,
	}, nil
}

func normalizeVersion(value string) (string, error) {
	version := value
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	if !semver.IsValid(version) {
		return "", fmt.Errorf("invalid semantic version")
	}
	return version, nil
}
