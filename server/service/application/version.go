package application

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"NanoKVM-Server/proto"
	"NanoKVM-Server/utils"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

type Latest struct {
	ManifestVersion   int    `json:"manifest_version,omitempty"`
	Version           string `json:"version"`
	Name              string `json:"name"`
	Sha512            string `json:"sha512"`
	LegacySize        uint64 `json:"size"`
	SizeBytes         uint64 `json:"size_bytes,omitempty"`
	UnpackedSizeBytes uint64 `json:"unpacked_size_bytes,omitempty"`
	Url               string `json:"-"`
}

const (
	maxLatestJSONSize = 64 * 1024
)

var (
	latestClient       = utils.NewUpdateHTTPClient(15 * time.Second)
	packageNamePattern = regexp.MustCompile(`^nanokvm_[0-9]+\.[0-9]+\.[0-9]+\.tar\.gz$`)
	versionPattern     = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
)

func (s *Service) GetVersion(c *gin.Context) {
	var rsp proto.Response

	// current version
	current := currentVersion()

	log.Debugf("current version: %s", current)

	// latest version
	latestVersion := ""
	latest, err := getLatest()
	if err != nil {
		log.Errorf("failed to get latest version: %s", err)
		rsp.ErrRsp(c, -1, "failed to query latest version")
		return
	}
	latestVersion = latest.Version

	rsp.OkRspWithData(c, &proto.GetVersionRsp{
		Current: current,
		Latest:  latestVersion,
	})
}

// currentVersion reads the installed version, defaulting to 1.0.0 for a fresh or legacy box.
func currentVersion() string {
	version := "1.0.0"

	versionFile := fmt.Sprintf("%s/version", AppDir)
	if data, err := os.ReadFile(versionFile); err == nil {
		version = strings.ReplaceAll(string(data), "\n", "")
	}

	return version
}

// checkNotRollback refuses an update whose version is not newer than the installed one.
// allow=true is an explicit downgrade/reinstall and skips the check entirely, even when
// the installed version is unparseable; the latest version is already validated by validateLatest.
func checkNotRollback(current, latest string, allow bool) error {
	if allow {
		return nil
	}
	cmp, err := compareVersions(latest, current)
	if err != nil {
		return fmt.Errorf("refusing update: %w", err)
	}
	if cmp <= 0 {
		return fmt.Errorf("refusing update: latest %s is not newer than installed %s", latest, current)
	}
	return nil
}

// compareVersions orders two versions accepted by versionPattern per semver 2.0:
// -1 if a < b, 0 if equal, 1 if a > b. Build metadata is ignored.
func compareVersions(a, b string) (int, error) {
	coreA, preA, err := parseVersion(a)
	if err != nil {
		return 0, err
	}
	coreB, preB, err := parseVersion(b)
	if err != nil {
		return 0, err
	}

	for i := range coreA {
		if coreA[i] != coreB[i] {
			return cmpUint(coreA[i], coreB[i]), nil
		}
	}

	// A prerelease sorts below the release it precedes.
	switch {
	case len(preA) == 0 && len(preB) == 0:
		return 0, nil
	case len(preA) == 0:
		return 1, nil
	case len(preB) == 0:
		return -1, nil
	}
	for i := 0; i < len(preA) && i < len(preB); i++ {
		if c := comparePrerelease(preA[i], preB[i]); c != 0 {
			return c, nil
		}
	}
	return cmpUint(uint64(len(preA)), uint64(len(preB))), nil
}

func parseVersion(v string) (core [3]uint64, pre []string, err error) {
	if !versionPattern.MatchString(v) {
		return core, nil, fmt.Errorf("invalid version %q", v)
	}
	v, _, _ = strings.Cut(v, "+")
	v, prerelease, hasPre := strings.Cut(v, "-")
	for i, part := range strings.Split(v, ".") {
		n, perr := strconv.ParseUint(part, 10, 64)
		if perr != nil {
			return core, nil, fmt.Errorf("invalid version %q: %w", v, perr)
		}
		core[i] = n
	}
	if hasPre {
		pre = strings.Split(prerelease, ".")
		for _, id := range pre {
			if id == "" {
				return core, nil, fmt.Errorf("invalid version %q: empty prerelease identifier", v)
			}
		}
	}
	return core, pre, nil
}

// comparePrerelease: numeric identifiers compare numerically and sort below alphanumeric ones.
func comparePrerelease(a, b string) int {
	na, errA := strconv.ParseUint(a, 10, 64)
	nb, errB := strconv.ParseUint(b, 10, 64)
	switch {
	case errA == nil && errB == nil:
		return cmpUint(na, nb)
	case errA == nil:
		return -1
	case errB == nil:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

func cmpUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func getLatest() (*Latest, error) {
	baseURL, err := resolveUpdateBaseURL()
	if err != nil {
		return nil, err
	}

	manifestURL, err := joinUpdateURL(baseURL, "latest.json")
	if err != nil {
		return nil, err
	}
	parsedManifestURL, err := url.Parse(manifestURL)
	if err != nil {
		return nil, err
	}
	query := parsedManifestURL.Query()
	query.Set("now", fmt.Sprintf("%d", time.Now().Unix()))
	parsedManifestURL.RawQuery = query.Encode()

	request, err := utils.NewAuthenticatedRequest("GET", parsedManifestURL.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := latestClient.Do(request)
	if err != nil {
		log.Debugf("failed to request version from %s", parsedManifestURL.Redacted())
		return nil, errors.New("update server is inaccessible")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLatestJSONSize+1))
	if err != nil {
		log.Errorf("failed to read response: %v", err)
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		log.Errorf("server responded with status code: %d", resp.StatusCode)
		return nil, fmt.Errorf("status code %d", resp.StatusCode)
	}
	if len(body) > maxLatestJSONSize {
		return nil, fmt.Errorf("latest manifest exceeds %d bytes", maxLatestJSONSize)
	}

	var latest Latest
	if err := json.Unmarshal(body, &latest); err != nil {
		log.Errorf("failed to unmarshal response: %s", err)
		return nil, err
	}
	if err := validateLatest(&latest); err != nil {
		return nil, err
	}

	latest.Url, err = joinUpdateURL(baseURL, latest.Name)
	if err != nil {
		return nil, err
	}

	log.Debugf("get application latest version: %s", latest.Version)
	return &latest, nil
}

func joinUpdateURL(baseURL string, element string) (string, error) {
	joined, err := url.JoinPath(baseURL, element)
	if err != nil {
		return "", fmt.Errorf("join update URL: %w", err)
	}
	return joined, nil
}

func validateLatest(latest *Latest) error {
	if !versionPattern.MatchString(latest.Version) {
		return errors.New("invalid latest version")
	}
	if !packageNamePattern.MatchString(latest.Name) {
		return errors.New("invalid update package name")
	}
	// The signature names the version, so the package must be that version's.
	if latest.Name != "nanokvm_"+latest.Version+".tar.gz" {
		return errors.New("update package name does not match its version")
	}
	digest, err := base64.StdEncoding.DecodeString(latest.Sha512)
	if err != nil || len(digest) != 64 {
		return errors.New("invalid update package sha512")
	}
	if latest.LegacySize == 0 {
		return errors.New("invalid update package size")
	}
	switch latest.ManifestVersion {
	case 0, 1:
		return nil
	case 2:
		if latest.SizeBytes == 0 || latest.SizeBytes > maxPackageSize {
			return errors.New("invalid update package size_bytes")
		}
		if latest.UnpackedSizeBytes == 0 || latest.UnpackedSizeBytes > maxExpandedSize {
			return errors.New("invalid update package unpacked_size_bytes")
		}
		return nil
	default:
		return errors.New("unsupported update manifest version")
	}
}

func preflightManifestSpace(path string, latest *Latest) error {
	if latest.ManifestVersion != 2 {
		return nil
	}
	return ensureFreeSpace(path, latest.SizeBytes)
}

func validateDownloadedSize(latest *Latest, written uint64) error {
	if written > maxPackageSize {
		return fmt.Errorf("update package exceeds %d bytes", maxPackageSize)
	}
	if latest.ManifestVersion == 2 && written != latest.SizeBytes {
		return fmt.Errorf("update package size mismatch: expected %d bytes, got %d", latest.SizeBytes, written)
	}
	return nil
}

func validateExpandedSize(latest *Latest, expanded uint64) error {
	if latest.ManifestVersion == 2 && expanded != latest.UnpackedSizeBytes {
		return fmt.Errorf("update package expanded size mismatch: expected %d bytes, got %d", latest.UnpackedSizeBytes, expanded)
	}
	return nil
}
