package services

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"elearning-sbt/config"
	"elearning-sbt/version"
)

// UpdateCheckResult represents the outcome of checking for a newer release.
type UpdateCheckResult struct {
	Configured      bool   `json:"configured"`
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseNotes    string `json:"release_notes,omitempty"`
	DownloadURL     string `json:"download_url,omitempty"`
	SHA256          string `json:"sha256,omitempty"`
	HTMLURL         string `json:"html_url,omitempty"`
	PublishedAt     string `json:"published_at,omitempty"`
	AssetName       string `json:"asset_name,omitempty"`
	Message         string `json:"message,omitempty"`
}

// ApplyResult describes the outcome of applying an update before restart.
type ApplyResult struct {
	Status     string `json:"status"`
	Message    string `json:"message"`
	BackupPath string `json:"backup_path,omitempty"`
	SHA256     string `json:"sha256"`
}

// UpdaterService manages release discovery, database backup, and safe binary replacement.
type UpdaterService struct {
	cfg        *config.Config
	httpClient *http.Client
}

// NewUpdaterService creates a new UpdaterService with sensible HTTP timeouts.
func NewUpdaterService(cfg *config.Config) *UpdaterService {
	return &UpdaterService{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CleanupOldBinary removes leftover .old and .new executables from a previous self-update.
func CleanupOldBinary() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	_ = os.Remove(exePath + ".old")
	_ = os.Remove(exePath + ".new")
}

// CheckForUpdate queries the configured custom manifest URL or GitHub Releases API.
func (s *UpdaterService) CheckForUpdate(ctx context.Context) (*UpdateCheckResult, error) {
	res := &UpdateCheckResult{
		CurrentVersion: version.Version,
	}

	if s.cfg.UpdateManifestURL != "" {
		res.Configured = true
		return s.checkCustomManifest(ctx, res)
	}
	if s.cfg.UpdateRepo != "" {
		res.Configured = true
		return s.checkGitHubRelease(ctx, res)
	}

	res.Configured = false
	res.LatestVersion = version.Version
	res.Message = "ยังไม่ได้ตั้งค่า UPDATE_REPO หรือ UPDATE_MANIFEST_URL ในไฟล์ .env"
	return res, nil
}

type customManifest struct {
	Version      string `json:"version"`
	DownloadURL  string `json:"download_url"`
	SHA256       string `json:"sha256"`
	ReleaseNotes string `json:"release_notes"`
	HTMLURL      string `json:"html_url"`
	PublishedAt  string `json:"published_at"`
	AssetName    string `json:"asset_name"`
}

func (s *UpdaterService) checkCustomManifest(ctx context.Context, res *UpdateCheckResult) (*UpdateCheckResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.UpdateManifestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "LearnChain-Updater/"+version.Version)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch update manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update manifest returned HTTP %d", resp.StatusCode)
	}

	var m customManifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("invalid update manifest JSON: %w", err)
	}

	res.LatestVersion = m.Version
	res.DownloadURL = m.DownloadURL
	res.SHA256 = strings.ToLower(strings.TrimSpace(m.SHA256))
	res.ReleaseNotes = m.ReleaseNotes
	res.HTMLURL = m.HTMLURL
	res.PublishedAt = m.PublishedAt
	res.AssetName = m.AssetName
	res.UpdateAvailable = version.IsNewer(version.Version, m.Version)
	return res, nil
}

type githubRelease struct {
	TagName     string `json:"tag_name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func (s *UpdaterService) checkGitHubRelease(ctx context.Context, res *UpdateCheckResult) (*UpdateCheckResult, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", strings.Trim(s.cfg.UpdateRepo, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "LearnChain-Updater/"+version.Version)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query GitHub releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub release JSON: %w", err)
	}

	var exeURL, exeName, checksumURL string
	bestScore := -1
	for _, asset := range rel.Assets {
		lower := strings.ToLower(asset.Name)
		if lower == "checksums.txt" || lower == "sha256sums.txt" {
			checksumURL = asset.BrowserDownloadURL
			continue
		}
		if strings.HasSuffix(lower, ".exe") {
			score := scoreExeAsset(lower, runtime.GOARCH)
			if score > bestScore {
				bestScore = score
				exeURL = asset.BrowserDownloadURL
				exeName = asset.Name
			}
		}
	}

	var sha string
	if checksumURL != "" && exeName != "" {
		sha, _ = s.fetchAssetChecksum(ctx, checksumURL, exeName)
	}

	res.LatestVersion = rel.TagName
	res.ReleaseNotes = rel.Body
	res.HTMLURL = rel.HTMLURL
	res.PublishedAt = rel.PublishedAt
	res.DownloadURL = exeURL
	res.AssetName = exeName
	res.SHA256 = sha
	res.UpdateAvailable = version.IsNewer(version.Version, rel.TagName)
	return res, nil
}

func (s *UpdaterService) fetchAssetChecksum(ctx context.Context, checksumURL, assetName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "LearnChain-Updater/"+version.Version)

	resp, err := s.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return "", errors.New("checksum file unavailable")
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 {
			hash := strings.ToLower(strings.TrimSpace(fields[0]))
			name := strings.TrimPrefix(strings.TrimSpace(fields[1]), "*")
			if strings.EqualFold(filepath.Base(name), assetName) && len(hash) == 64 {
				return hash, nil
			}
		}
	}
	return "", errors.New("asset checksum not found")
}

// BackupDatabase copies the SQLite database into ./backups/ before an upgrade.
func (s *UpdaterService) BackupDatabase(dbPath string) (string, error) {
	if dbPath == "" {
		return "", nil
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return "", nil
	}

	if err := os.MkdirAll("./backups", 0755); err != nil {
		return "", fmt.Errorf("failed to create backups directory: %w", err)
	}

	safeVer := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(version.Version)
	backupName := fmt.Sprintf("elearning_%s_%s.db", safeVer, time.Now().Format("20060102_150405"))
	backupPath := filepath.Join("./backups", backupName)

	src, err := os.Open(dbPath)
	if err != nil {
		return "", fmt.Errorf("failed to open database for backup: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		return "", fmt.Errorf("failed to create database backup: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("failed to write database backup: %w", err)
	}

	return backupPath, nil
}

// ApplyUpdate downloads the new executable, verifies SHA-256 (if provided),
// backs up the database, swaps the running binary safely, and launches the new version.
func (s *UpdaterService) ApplyUpdate(ctx context.Context, downloadURL, expectedSHA256 string) (*ApplyResult, error) {
	if !isAllowedDownloadURL(downloadURL) {
		return nil, errors.New("invalid download URL: must use https://")
	}

	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("cannot locate current executable: %w", err)
	}

	newPath := exePath + ".new"
	oldPath := exePath + ".old"

	// Use a dedicated client with longer timeout for binary downloads.
	dlClient := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "LearnChain-Updater/"+version.Version)

	resp, err := dlClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download server returned HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(newPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stage new binary: %w", err)
	}

	hasher := sha256.New()
	writer := io.MultiWriter(out, hasher)
	_, copyErr := io.Copy(writer, resp.Body)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(newPath)
		return nil, fmt.Errorf("failed to write update binary: %v", copyErr)
	}

	actualSHA256 := hex.EncodeToString(hasher.Sum(nil))
	expected := strings.ToLower(strings.TrimSpace(expectedSHA256))
	if expected != "" && actualSHA256 != expected {
		_ = os.Remove(newPath)
		return nil, fmt.Errorf("SHA-256 checksum mismatch (expected %s, got %s)", expected, actualSHA256)
	}

	// Backup SQLite database before swapping binaries.
	backupPath, err := s.BackupDatabase(s.cfg.DBPath)
	if err != nil {
		_ = os.Remove(newPath)
		return nil, err
	}

	// Windows-safe binary swap: rename running exe to .old, then move .new into place.
	_ = os.Remove(oldPath)
	if err := os.Rename(exePath, oldPath); err != nil {
		_ = os.Remove(newPath)
		return nil, fmt.Errorf("failed to move current binary aside: %w", err)
	}
	if err := os.Rename(newPath, exePath); err != nil {
		// Rollback to previous binary
		_ = os.Rename(oldPath, exePath)
		return nil, fmt.Errorf("failed to activate new binary: %w", err)
	}

	// Launch the newly updated executable; it will clean up .old on startup.
	cmd := exec.Command(exePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("update installed, but failed to restart automatically: %w", err)
	}

	return &ApplyResult{
		Status:     "updating",
		Message:    "อัปเดตสำเร็จ กำลังรีสตาร์ทโปรแกรมเป็นเวอร์ชันใหม่...",
		BackupPath: backupPath,
		SHA256:     actualSHA256,
	}, nil
}

func scoreExeAsset(lowerName, arch string) int {
	score := 0
	if strings.Contains(lowerName, "learnchain") {
		score += 10
	}
	isArm64 := strings.Contains(lowerName, "arm64")
	isX64 := strings.Contains(lowerName, "amd64") || strings.Contains(lowerName, "x64")

	switch arch {
	case "arm64":
		if isArm64 {
			score += 100
		} else if !isX64 {
			score += 20 // generic LearnChain.exe fallback
		}
	case "amd64":
		if isX64 {
			score += 100
		} else if !isArm64 {
			score += 50 // generic LearnChain.exe (built as amd64)
		}
	}
	return score
}

func isAllowedDownloadURL(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "http://localhost") ||
		strings.HasPrefix(lower, "http://127.0.0.1")
}
