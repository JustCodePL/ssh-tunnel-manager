package updater

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckStableIgnoresPrereleasesAndInvalidReleases(t *testing.T) {
	releases := []map[string]any{
		testRelease("v1.2.0-beta.2", true, false, true),
		testRelease("not-a-version", false, false, true),
		testRelease("v9.0.0", false, true, true),
		testRelease("v0.9.0", false, false, true),
		testRelease("v1.1.0", false, false, true),
	}

	info, err := checkTestReleases(t, "1.0.0", ChannelStable, releases)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if info == nil || info.LatestVersion != "1.1.0" {
		t.Fatalf("latest version = %#v, want 1.1.0", info)
	}
}

func TestCheckBetaSelectsHighestSemverRegardlessOfAPIOrder(t *testing.T) {
	releases := []map[string]any{
		testRelease("v1.3.0-beta.1", true, false, true),
		testRelease("v1.1.0", false, false, true),
		testRelease("v1.2.0-beta.9", true, false, true),
	}

	info, err := checkTestReleases(t, "1.0.0", ChannelBeta, releases)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if info == nil || info.LatestVersion != "1.3.0-beta.1" {
		t.Fatalf("latest version = %#v, want 1.3.0-beta.1", info)
	}
}

func TestCheckSemverTransitions(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		channel  Channel
		releases []map[string]any
		want     string
	}{
		{
			name:     "next beta",
			current:  "1.2.0-beta.1",
			channel:  ChannelBeta,
			releases: []map[string]any{testRelease("v1.2.0-beta.2", true, false, true)},
			want:     "1.2.0-beta.2",
		},
		{
			name:     "beta to final on beta channel",
			current:  "1.2.0-beta.2",
			channel:  ChannelBeta,
			releases: []map[string]any{testRelease("v1.2.0", false, false, true)},
			want:     "1.2.0",
		},
		{
			name:     "beta to final on stable channel",
			current:  "1.2.0-beta.2",
			channel:  ChannelStable,
			releases: []map[string]any{testRelease("v1.2.0", false, false, true)},
			want:     "1.2.0",
		},
		{
			name:     "no downgrade from final to beta",
			current:  "1.2.0",
			channel:  ChannelBeta,
			releases: []map[string]any{testRelease("v1.2.0-beta.3", true, false, true)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := checkTestReleases(t, tt.current, tt.channel, tt.releases)
			if err != nil {
				t.Fatalf("Check returned error: %v", err)
			}
			if tt.want == "" {
				if info != nil {
					t.Fatalf("update = %#v, want nil", info)
				}
				return
			}
			if info == nil || info.LatestVersion != tt.want {
				t.Fatalf("latest version = %#v, want %s", info, tt.want)
			}
		})
	}
}

func TestCheckStableAllowsControlledDowngradeFromPrerelease(t *testing.T) {
	releases := []map[string]any{
		testRelease("v1.2.0-beta.1", true, false, true),
		testRelease("v1.1.1", false, false, true),
		testRelease("v1.1.0", false, false, true),
	}

	info, err := checkTestReleases(t, "1.2.0-beta.1", ChannelStable, releases)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if info == nil || info.LatestVersion != "1.1.1" {
		t.Fatalf("latest version = %#v, want 1.1.1", info)
	}
}

func TestCheckStableDoesNotDowngradeStableBuild(t *testing.T) {
	releases := []map[string]any{
		testRelease("v1.1.1", false, false, true),
		testRelease("v1.1.0", false, false, true),
	}

	info, err := checkTestReleases(t, "1.2.0", ChannelStable, releases)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if info != nil {
		t.Fatalf("update = %#v, want nil", info)
	}
}

func TestCheckReportsMissingAssetOnHighestEligibleRelease(t *testing.T) {
	releases := []map[string]any{
		testRelease("v1.2.0-beta.2", true, false, false),
		testRelease("v1.2.0-beta.1", true, false, true),
	}

	info, err := checkTestReleases(t, "1.0.0", ChannelBeta, releases)
	if info != nil {
		t.Fatalf("update = %#v, want nil", info)
	}
	if err == nil || !strings.Contains(err.Error(), "no asset named") {
		t.Fatalf("error = %v, want missing asset error", err)
	}
}

func TestCheckRejectsInvalidInputs(t *testing.T) {
	if _, err := check(context.Background(), "bad-version", "owner/project", ChannelStable, "http://unused", http.DefaultClient); err == nil {
		t.Fatal("invalid current version should return an error")
	}
	if _, err := check(context.Background(), "1.0.0", "owner/project", Channel("nightly"), "http://unused", http.DefaultClient); err == nil {
		t.Fatal("invalid channel should return an error")
	}
}

func TestCheckDevVersionSkipsRequest(t *testing.T) {
	info, err := check(context.Background(), "dev", "owner/project", ChannelStable, "http://unused", http.DefaultClient)
	if err != nil || info != nil {
		t.Fatalf("Check(dev) = %#v, %v; want nil, nil", info, err)
	}
}

func checkTestReleases(t *testing.T, current string, channel Channel, releases []map[string]any) (*UpdateInfo, error) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/project/releases" {
			t.Errorf("request path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("per_page"); got != "100" {
			t.Errorf("per_page = %q, want 100", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(releases); err != nil {
			t.Errorf("encoding releases: %v", err)
		}
	}))
	defer server.Close()

	return check(context.Background(), current, "owner/project", channel, server.URL, server.Client())
}

func testRelease(version string, prerelease, draft, withAsset bool) map[string]any {
	assets := []map[string]string{}
	if withAsset {
		assets = append(assets, map[string]string{
			"name":                 platformAsset,
			"browser_download_url": "https://example.test/" + platformAsset,
		})
	}
	return map[string]any{
		"tag_name":   version,
		"html_url":   "https://example.test/releases/" + version,
		"body":       "Release notes for " + version,
		"draft":      draft,
		"prerelease": prerelease,
		"assets":     assets,
	}
}
