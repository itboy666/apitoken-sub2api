//go:build unit

package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const combinedTestRepo = "example/apitoken-sub2api"

func combinedRelease(version string) *GitHubRelease {
	return &GitHubRelease{TagName: "v" + version, HTMLURL: "https://github.com/" + combinedTestRepo + "/releases/tag/v" + version}
}

func combinedTestService(client GitHubReleaseClient, cache UpdateCache) *UpdateService {
	return NewUpdateService(cache, client, "2.23.0-apitoken.8", "release", UpdateSource{Channel: "apitoken", Repository: combinedTestRepo, UpstreamVersion: "2.23.0"})
}

func TestCombinedUpdateRequiresDedicatedSource(t *testing.T) {
	for _, repository := range []string{"", githubRepo, "Wei-Shaw/sub2api", "https://github.com/example/site", "../evil", "owner/repo?token=x"} {
		t.Run(repository, func(t *testing.T) {
			client := &updateServiceGitHubClientStub{}
			svc := NewUpdateService(nil, client, "2.23.0-apitoken.8", "release", UpdateSource{Repository: repository})
			info, err := svc.CheckUpdate(t.Context(), true)
			require.NoError(t, err)
			require.Equal(t, "apitoken", info.UpdateChannel)
			require.Equal(t, "custom_repository_required", info.UpdateBlockedReason)
			require.False(t, info.AutoUpdateEnabled)
			require.Empty(t, info.LatestVersion, "unknown is not up to date")
			require.Empty(t, client.latestRepo, "must not query the official binary channel")
			require.ErrorIs(t, svc.PerformUpdate(t.Context()), ErrUpdateSourceRequired)
			require.ErrorIs(t, svc.RollbackToVersion(t.Context(), "2.23.0-apitoken.7"), ErrUpdateSourceRequired)
		})
	}
}

func TestCombinedUpdateRevisionAndSourceIsolation(t *testing.T) {
	client := &updateServiceGitHubClientStub{release: combinedRelease("2.23.0-apitoken.10")}
	cache := &updateServiceCacheStub{}
	// Simulate a Redis value left by the official installation.
	official := NewUpdateService(cache, &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v9.0.0"}}, "2.23.0", "release")
	_, err := official.CheckUpdate(t.Context(), true)
	require.NoError(t, err)
	svc := combinedTestService(client, cache)
	info, err := svc.CheckUpdate(t.Context(), false)
	require.NoError(t, err)
	require.Equal(t, combinedTestRepo, client.latestRepo)
	require.True(t, info.HasUpdate)
	require.True(t, info.AutoUpdateEnabled)
	require.Equal(t, "2.23.0-apitoken.10", info.LatestVersion)
	require.Equal(t, "2.23.0", info.UpstreamVersion)
	info, err = svc.CheckUpdate(t.Context(), false)
	require.NoError(t, err)
	require.True(t, info.Cached)
	// A repository switch must also invalidate an existing custom cache.
	svc.source.Repository = "different/site"
	_, err = svc.getFromCache(t.Context())
	require.Error(t, err)
	for _, pair := range [][2]string{{"2.23.0-apitoken.9", "2.23.0-apitoken.10"}, {"2.23.0-apitoken.10", "2.24.0-apitoken.1"}} {
		require.Less(t, compareVersions(pair[0], pair[1]), 0)
		require.Greater(t, compareVersions(pair[1], pair[0]), 0)
	}
}

func TestCombinedUpdateRejectsOfficialAndWrongRepositoryReleases(t *testing.T) {
	for _, release := range []*GitHubRelease{
		nil,
		{TagName: "v2.24.0"},
		combinedRelease("2.23.0-apitoken.7"),
		{TagName: "v2.24.0-apitoken.1", HTMLURL: "https://github.com/attacker/site/releases/tag/v2.24.0-apitoken.1"},
		{TagName: "v2.24.0-apitoken.1", HTMLURL: "https://github.com/" + combinedTestRepo + "/releases/tag/v2.24.0-apitoken.1", Prerelease: true},
	} {
		svc := combinedTestService(&updateServiceGitHubClientStub{release: release}, nil)
		info, err := svc.CheckUpdate(t.Context(), true)
		require.NoError(t, err)
		require.NotEmpty(t, info.Warning)
		require.False(t, info.HasUpdate)
		require.ErrorContains(t, svc.PerformUpdate(t.Context()), "verification unavailable")
	}
}

func TestCombinedUpdateDoesNotInstallStaleCacheWhenOffline(t *testing.T) {
	client := &updateInstallClient{}
	client.release = combinedRelease("2.23.0-apitoken.9")
	svc := combinedTestService(client, &updateServiceCacheStub{})
	_, err := svc.CheckUpdate(t.Context(), true)
	require.NoError(t, err)
	client.latestErr = errors.New("network unavailable")
	info, err := svc.CheckUpdate(t.Context(), true)
	require.NoError(t, err)
	require.True(t, info.Cached)
	require.True(t, info.HasUpdate)
	require.NotEmpty(t, info.Warning)
	require.ErrorContains(t, svc.PerformUpdate(t.Context()), "network unavailable")
}

func TestCombinedRollbackExcludesIncompatibleAndOfficialBuilds(t *testing.T) {
	client := &updateServiceGitHubClientStub{recentReleases: []*GitHubRelease{
		{TagName: "v2.24.0"}, combinedRelease("2.23.0-apitoken.10"), combinedRelease("2.23.0-apitoken.9"), combinedRelease("2.23.0-apitoken.8"), combinedRelease("2.23.0-apitoken.7"), combinedRelease("2.23.0-apitoken.6"), combinedRelease("2.22.0-apitoken.20"),
	}}
	svc := combinedTestService(client, nil)
	svc.currentVersion = "2.23.0-apitoken.11"
	versions, err := svc.ListRollbackVersions(t.Context())
	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "2.23.0-apitoken.10", versions[0].Version)
	require.Equal(t, "2.23.0-apitoken.8", versions[2].Version)
	require.ErrorIs(t, svc.RollbackToVersion(t.Context(), "2.23.0-apitoken.6"), ErrRollbackVersionNotAllowed)
	require.ErrorIs(t, svc.Rollback(), ErrCustomLocalRollback)
}

func TestSourceBuildCannotBypassUIUpdateRestriction(t *testing.T) {
	svc := NewUpdateService(nil, &updateServiceGitHubClientStub{}, "2.23.0", "source")
	require.ErrorIs(t, svc.PerformUpdate(context.Background()), ErrSourceBuildUpdate)
	require.ErrorIs(t, svc.Rollback(), ErrSourceBuildUpdate)
}

func combinedArchiveBytes(t *testing.T, version string, mutate func(map[string]string)) ([]byte, []byte) {
	t.Helper()
	binary := []byte("combined binary fixture")
	digest := sha256.Sum256(binary)
	manifest := map[string]string{"version": version, "repository": combinedTestRepo, "upstream_version": "2.23.0", "customization_abi": apitokenReleaseABI, "binary_sha256": hex.EncodeToString(digest[:])}
	if mutate != nil {
		mutate(manifest)
	}
	raw, err := json.Marshal(manifest)
	require.NoError(t, err)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tarball := tar.NewWriter(gz)
	for name, contents := range map[string][]byte{"sub2api": binary, "apitoken-release.json": raw} {
		require.NoError(t, tarball.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(contents))}))
		_, err = tarball.Write(contents)
		require.NoError(t, err)
	}
	require.NoError(t, tarball.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes(), binary
}

func TestCombinedArchiveProvenanceBeforeReplacement(t *testing.T) {
	for _, field := range []string{"", "version", "repository", "upstream_version", "customization_abi", "binary_sha256"} {
		t.Run(field, func(t *testing.T) {
			archive, binary := combinedArchiveBytes(t, "2.23.0-apitoken.9", func(manifest map[string]string) {
				if field != "" {
					manifest[field] = "wrong"
				}
			})
			dir := t.TempDir()
			archivePath, binaryPath := filepath.Join(dir, "release.tar.gz"), filepath.Join(dir, "sub2api")
			require.NoError(t, os.WriteFile(archivePath, archive, 0600))
			require.NoError(t, os.WriteFile(binaryPath, binary, 0600))
			err := combinedTestService(nil, nil).verifyCombinedArchive(archivePath, binaryPath, "2.23.0-apitoken.9")
			if field == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestCombinedUpdateRequiresChecksumAndRepositoryAssets(t *testing.T) {
	svc := combinedTestService(nil, nil)
	version := "2.23.0-apitoken.9"
	name := "sub2api_" + version + "_" + svc.getArchiveName() + ".tar.gz"
	assets := []Asset{{Name: name, DownloadURL: "https://github.com/" + combinedTestRepo + "/releases/download/v" + version + "/" + name}}
	require.ErrorContains(t, svc.applyReleaseAssets(t.Context(), assets, version), "require checksums.txt")
	assets = append(assets, Asset{Name: "checksums.txt", DownloadURL: "https://github.com/attacker/site/releases/download/v" + version + "/checksums.txt"})
	require.ErrorContains(t, svc.applyReleaseAssets(t.Context(), assets, version), "does not belong")
}
