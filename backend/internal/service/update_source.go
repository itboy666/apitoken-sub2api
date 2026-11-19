package service

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	apitokenUpdateChannel = "apitoken"
	// Older builds cannot safely display or process invoices issued with
	// independently configured pricing after migration 390.
	minimumAPITokenUpdate = "2.23.0-apitoken.9"
	apitokenReleaseABI    = "apitoken-v1"
)

var (
	ErrUpdateSourceRequired  = infraerrors.Conflict("UPDATE_SOURCE_REQUIRED", "请先配置合并版发布仓库，保留定制功能后再更新")
	ErrSourceBuildUpdate     = infraerrors.Conflict("SOURCE_BUILD_UPDATE", "源码构建请合并上游后重新编译，或安装合并版发布包")
	ErrCustomLocalRollback   = infraerrors.Conflict("CUSTOM_ROLLBACK_REQUIRES_RELEASE", "合并版请从兼容版本列表回退；本地旧备份未经过兼容性验证")
	releaseRepositoryPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9][A-Za-z0-9._-]*$`)
	apitokenVersionPattern   = regexp.MustCompile(`^v?([0-9]+\.[0-9]+\.[0-9]+)-apitoken\.([1-9][0-9]*)$`)
)

// UpdateSource is embedded by the combined-release builder. Repository can be
// supplied through the server environment when first connecting a private fork.
// It is never taken from an HTTP request or a downloaded release.
type UpdateSource struct {
	Channel         string
	Repository      string
	UpstreamVersion string
}

func normalizeUpdateSource(version string, sources []UpdateSource) UpdateSource {
	var source UpdateSource
	if len(sources) > 0 {
		source = sources[0]
	}
	source.Repository = strings.TrimSpace(source.Repository)
	source.UpstreamVersion = strings.TrimPrefix(strings.TrimSpace(source.UpstreamVersion), "v")
	if source.Channel == "" {
		source.Channel = "official"
	}
	// A missing build flag must never turn a custom build into an official
	// binary-replacement channel.
	if strings.Contains(version, "-apitoken.") {
		source.Channel = apitokenUpdateChannel
	}
	if source.Channel == "official" {
		source.Repository = githubRepo
	}
	if source.UpstreamVersion == "" {
		source.UpstreamVersion = strings.SplitN(strings.TrimPrefix(version, "v"), "-", 2)[0]
	}
	return source
}

func (s *UpdateService) isCustomDistribution() bool {
	return s.source.Channel == apitokenUpdateChannel
}

func (s *UpdateService) sourceConfigured() bool {
	if !releaseRepositoryPattern.MatchString(s.source.Repository) {
		return false
	}
	if s.isCustomDistribution() {
		return !strings.EqualFold(s.source.Repository, githubRepo) &&
			!strings.EqualFold(s.source.Repository, "Wei-Shaw/sub2api")
	}
	return s.source.Channel == "official"
}

func (s *UpdateService) updateBlockedReason() string {
	if !s.sourceConfigured() {
		return "custom_repository_required"
	}
	if s.buildType != "release" {
		return "source_build"
	}
	return ""
}

func (s *UpdateService) checkUpdateSource() error {
	if !s.sourceConfigured() {
		return ErrUpdateSourceRequired
	}
	if s.buildType != "release" {
		return ErrSourceBuildUpdate
	}
	return nil
}

func (s *UpdateService) withSourceInfo(info *UpdateInfo) *UpdateInfo {
	info.CurrentVersion = s.currentVersion
	info.BuildType = s.buildType
	info.UpdateChannel = s.source.Channel
	info.UpdateRepository = s.source.Repository
	info.UpstreamVersion = s.source.UpstreamVersion
	info.UpdateBlockedReason = s.updateBlockedReason()
	info.AutoUpdateEnabled = info.UpdateBlockedReason == ""
	return info
}

func apitokenRevision(version string) (int, bool) {
	m := apitokenVersionPattern.FindStringSubmatch(version)
	if m == nil {
		return 0, false
	}
	revision, err := strconv.Atoi(m[2])
	return revision, err == nil
}

func (s *UpdateService) validateRelease(release *GitHubRelease) error {
	if release == nil || release.Draft || release.Prerelease || strings.TrimSpace(release.TagName) == "" {
		return fmt.Errorf("release is missing or is not a stable release")
	}
	if !s.isCustomDistribution() {
		return nil
	}
	if _, ok := apitokenRevision(release.TagName); !ok || compareVersions(release.TagName, minimumAPITokenUpdate) < 0 {
		return fmt.Errorf("release is not a compatible APIToken combined build")
	}
	return s.validateRepositoryURL(release.HTMLURL, "/releases/tag/"+release.TagName)
}

func (s *UpdateService) validateRepositoryURL(raw, suffix string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("release URL must belong to the configured GitHub repository")
	}
	expected := "/" + s.source.Repository + suffix
	if !strings.EqualFold(strings.TrimSuffix(u.Path, suffix), "/"+s.source.Repository) || !strings.HasSuffix(u.Path, suffix) || len(u.Path) != len(expected) {
		return fmt.Errorf("release URL does not belong to %s", s.source.Repository)
	}
	return nil
}

// A repository can also contain upstream release tags. Check the package's
// provenance and binary digest before any rename, even when the archive checksum
// is valid. Official archives have no such contract and cannot replace this host.
func (s *UpdateService) verifyCombinedArchive(archivePath, binaryPath, version string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var raw []byte
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Name != "apitoken-release.json" {
			continue
		}
		if raw != nil || hdr.Typeflag != tar.TypeReg || hdr.Size > 16*1024 {
			return fmt.Errorf("invalid combined-release manifest")
		}
		raw, err = io.ReadAll(io.LimitReader(tr, 16*1024+1))
		if err != nil {
			return err
		}
	}
	var manifest struct {
		Version         string `json:"version"`
		Repository      string `json:"repository"`
		UpstreamVersion string `json:"upstream_version"`
		ABI             string `json:"customization_abi"`
		BinarySHA256    string `json:"binary_sha256"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return fmt.Errorf("missing or invalid combined-release manifest: %w", err)
	}
	upstream := strings.SplitN(version, "-", 2)[0]
	if manifest.Version != version || !strings.EqualFold(manifest.Repository, s.source.Repository) || manifest.UpstreamVersion != upstream || manifest.ABI != apitokenReleaseABI {
		return fmt.Errorf("combined-release provenance does not match the selected release")
	}
	binary, err := os.Open(binaryPath)
	if err != nil {
		return err
	}
	defer binary.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, binary); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != manifest.BinarySHA256 {
		return fmt.Errorf("combined-release binary digest mismatch")
	}
	return nil
}
