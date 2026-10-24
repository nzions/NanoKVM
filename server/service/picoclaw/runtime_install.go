package picoclaw

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"

	"NanoKVM-Server/service/application"
	"NanoKVM-Server/service/updatesign"
	"NanoKVM-Server/utils"
)

func (s *Service) installRuntime() (string, *PicoclawError) {
	if s == nil {
		return "", newPicoclawError(CodeRuntimeUnavailable, "picoclaw service is unavailable")
	}
	s.ensureDependencies()
	log.Debugf("picoclaw install: start, binary=%s, cache=%s", picoclawBinaryPath, picoclawCacheDir)

	currentStatus := s.runtime.Get()
	if currentStatus.Installing {
		log.Debugf("picoclaw install: install already in progress")
		return "picoclaw installation is already in progress", nil
	}

	if installed, err := isPicoclawInstalled(); err == nil && installed {
		settings, _ := loadPicoclawGatewaySettings()
		log.Debugf("picoclaw install: binary already exists at %s", picoclawBinaryPath)
		s.runtime.Set(RuntimeStatus{
			Ready:           false,
			Installed:       true,
			Installing:      false,
			InstallProgress: 100,
			InstallStage:    "installed",
			InstallPath:     picoclawBinaryPath,
			ModelConfigured: settings.ModelConfigured,
			ModelName:       settings.ModelName,
			Status:          "installed",
			CheckedAt:       time.Now(),
		})
		return "picoclaw is already installed", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), picoclawInstallTimeout)
	s.runtime.Set(RuntimeStatus{
		Ready:           false,
		Installed:       false,
		Installing:      true,
		InstallProgress: 0,
		InstallStage:    "preparing",
		InstallPath:     picoclawBinaryPath,
		Status:          "installing",
		CheckedAt:       time.Now(),
	})

	go s.runInstallRuntime(ctx, cancel)
	return "picoclaw installation started", nil
}

func (s *Service) runInstallRuntime(ctx context.Context, cancel context.CancelFunc) {
	s.ensureDependencies()
	defer cancel()

	_ = os.RemoveAll(picoclawCacheDir)
	if err := os.MkdirAll(picoclawCacheDir, 0o755); err != nil {
		log.Errorf("picoclaw install: failed to create cache directory %s: %v", picoclawCacheDir, err)
		s.finishInstallFailure("install_failed", fmt.Sprintf("failed to create cache directory: %v", err))
		return
	}
	log.Debugf("picoclaw install: cache directory ready at %s", picoclawCacheDir)
	defer func() {
		if err := os.RemoveAll(picoclawCacheDir); err != nil {
			log.Errorf("picoclaw install: failed to clean cache directory %s: %v", picoclawCacheDir, err)
			return
		}
		log.Debugf("picoclaw install: cleaned cache directory %s", picoclawCacheDir)
	}()

	s.setInstallProgress("downloading", 5, "")
	archiveURL, err := picoclawArchiveURL()
	if err != nil {
		log.Errorf("picoclaw install: no usable download URL: %v", err)
		s.finishInstallFailure("install_failed", err.Error())
		return
	}
	client := newPicoclawClient()

	archivePath := filepath.Join(picoclawCacheDir, "picoclaw.tar.gz")
	log.Debugf("picoclaw install: downloading archive to %s", archivePath)
	if err := downloadPicoclawArchive(ctx, client, archiveURL, archivePath, func(downloaded int64, total int64) {
		progress := 10
		if total > 0 {
			progress = 10 + int(float64(downloaded)*70/float64(total))
			if progress > 80 {
				progress = 80
			}
		}
		s.setInstallProgress("downloading", progress, "")
	}); err != nil {
		log.Errorf("picoclaw install: download failed: %v", err)
		s.finishInstallFailure(installFailureStatus(err), err.Error())
		return
	}
	log.Debugf("picoclaw install: archive download completed")

	s.setInstallProgress("verifying", 82, "")

	// The signature is the gate: nothing is extracted or installed before it passes.
	if err := verifyPicoclawSignature(client, archiveURL, archivePath); err != nil {
		log.Errorf("picoclaw install: signature verification failed: %v", err)
		s.finishInstallFailure(installFailureStatus(err), err.Error())
		return
	}
	log.Debugf("picoclaw install: archive signature verified for %s", archivePath)

	s.setInstallProgress("extracting", 85, "")
	log.Debugf("picoclaw install: extracting binary from %s", archivePath)
	extractedPath, err := extractPicoclawBinary(archivePath, picoclawCacheDir)
	if err != nil {
		log.Errorf("picoclaw install: extract failed: %v", err)
		s.finishInstallFailure(installFailureStatus(err), err.Error())
		return
	}
	log.Debugf("picoclaw install: extracted binary to %s", extractedPath)

	s.setInstallProgress("installing", 95, "")
	log.Debugf("picoclaw install: installing binary to %s", picoclawBinaryPath)
	if err := installPicoclawBinary(extractedPath, picoclawBinaryPath); err != nil {
		log.Errorf("picoclaw install: install failed: %v", err)
		s.finishInstallFailure(installFailureStatus(err), err.Error())
		return
	}
	log.Debugf("picoclaw install: install completed successfully")

	s.runtime.Set(RuntimeStatus{
		Ready:           false,
		Installed:       true,
		Installing:      false,
		InstallProgress: 100,
		InstallStage:    "installed",
		InstallPath:     picoclawBinaryPath,
		Status:          "installed",
		CheckedAt:       time.Now(),
	})
}

func downloadPicoclawArchive(ctx context.Context, client *http.Client, archiveURL string, destination string, onProgress func(downloaded int64, total int64)) error {
	req, err := utils.NewAuthenticatedRequest(http.MethodGet, archiveURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}
	req = req.WithContext(ctx)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download picoclaw: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download picoclaw: unexpected status %s", resp.Status)
	}

	if resp.ContentLength > picoclawMaxArchiveBytes {
		return fmt.Errorf("picoclaw archive exceeds %d bytes", picoclawMaxArchiveBytes)
	}

	file, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("failed to create archive file: %w", err)
	}
	defer file.Close()

	limited := io.LimitReader(resp.Body, picoclawMaxArchiveBytes+1)
	counter := &countingWriter{w: file}
	if err := copyWithProgress(ctx, counter, limited, resp.ContentLength, onProgress); err != nil {
		return fmt.Errorf("failed to save archive: %w", err)
	}
	if counter.n > picoclawMaxArchiveBytes {
		return fmt.Errorf("picoclaw archive exceeds %d bytes", picoclawMaxArchiveBytes)
	}
	return nil
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

func copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, total int64, onProgress func(downloaded int64, total int64)) error {
	buffer := make([]byte, 32*1024)
	var downloaded int64

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, readErr := src.Read(buffer)
		if n > 0 {
			if _, writeErr := dst.Write(buffer[:n]); writeErr != nil {
				return writeErr
			}
			downloaded += int64(n)
			if onProgress != nil {
				onProgress(downloaded, total)
			}
		}
		if readErr == io.EOF {
			if onProgress != nil {
				onProgress(downloaded, total)
			}
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func (s *Service) setInstallProgress(stage string, progress int, lastError string) {
	s.ensureDependencies()
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}

	s.runtime.UpdateInstallStatus(func(status *RuntimeStatus) {
		status.Ready = false
		status.Installed = false
		status.Installing = true
		status.InstallProgress = progress
		status.InstallStage = stage
		status.Status = "installing"
		status.LastError = lastError
		status.CheckedAt = time.Now()
	})
}

func (s *Service) finishInstallFailure(status string, message string) {
	s.ensureDependencies()
	s.runtime.Set(RuntimeStatus{
		Ready:           false,
		Installed:       false,
		Installing:      false,
		InstallProgress: 0,
		InstallStage:    status,
		InstallPath:     picoclawBinaryPath,
		Status:          status,
		LastError:       message,
		CheckedAt:       time.Now(),
	})
}

func installFailureStatus(err error) string {
	if err == nil {
		return "install_failed"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "install_timeout"
	}
	return "install_failed"
}

// picoclawArchiveURL is <update base>/picoclaw/v<version>/<archive>; the base
// is the configured update server and must be https.
func picoclawArchiveURL() (string, error) {
	base, err := application.ResolveUpdateBaseURL()
	if err != nil {
		return "", err
	}
	archiveURL, err := url.JoinPath(base, "picoclaw", "v"+picoclawVersion, picoclawArchiveName)
	if err != nil {
		return "", fmt.Errorf("join picoclaw URL: %w", err)
	}
	if err := updatesign.RequireHTTPS(archiveURL); err != nil {
		return "", err
	}
	return archiveURL, nil
}

// picoclawDisplayURL is the archive URL with any credentials redacted, for the UI.
func picoclawDisplayURL() string {
	archiveURL, err := picoclawArchiveURL()
	if err != nil {
		return ""
	}
	parsed, err := url.Parse(archiveURL)
	if err != nil {
		return ""
	}
	return parsed.Redacted()
}

func fileSHA256(path string) ([sha256.Size]byte, error) {
	var sum [sha256.Size]byte
	file, err := os.Open(path)
	if err != nil {
		return sum, fmt.Errorf("failed to open archive for hashing: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return sum, fmt.Errorf("failed to hash archive: %w", err)
	}
	copy(sum[:], hasher.Sum(nil))
	return sum, nil
}

// verifyPicoclawSignature requires <archive URL>.sig to carry a valid
// signature over picoclaw-<version>-riscv64 and the archive's sha256.
func verifyPicoclawSignature(client *http.Client, archiveURL string, archivePath string) error {
	sum, err := fileSHA256(archivePath)
	if err != nil {
		return err
	}
	envelope, err := updatesign.FetchEnvelope(client, archiveURL)
	if err != nil {
		return fmt.Errorf("picoclaw signature: %w", err)
	}
	signer, err := updatesign.Verify(updatesign.PicoclawName(picoclawVersion), envelope, sum)
	if err != nil {
		return fmt.Errorf("picoclaw signature: %w", err)
	}
	log.Infof("picoclaw %s signed by %s", picoclawVersion, signer)
	return nil
}

func newPicoclawClient() *http.Client {
	return utils.NewUpdateHTTPClient(picoclawDownloadTimeout)
}

func extractPicoclawBinary(archivePath string, destinationDir string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("failed to read archive: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to extract archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(header.Name) != "picoclaw" {
			continue
		}

		extractedPath := filepath.Join(destinationDir, "picoclaw")
		outFile, err := os.OpenFile(extractedPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return "", fmt.Errorf("failed to create extracted binary: %w", err)
		}
		if _, err := io.Copy(outFile, tarReader); err != nil {
			_ = outFile.Close()
			return "", fmt.Errorf("failed to extract picoclaw binary: %w", err)
		}
		if err := outFile.Close(); err != nil {
			return "", fmt.Errorf("failed to finalize extracted binary: %w", err)
		}
		return extractedPath, nil
	}

	return "", fmt.Errorf("picoclaw binary not found in archive")
}

func installPicoclawBinary(source string, destination string) error {
	inFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("failed to open extracted picoclaw binary: %w", err)
	}
	defer inFile.Close()

	tempDestination := destination + ".tmp"
	outFile, err := os.OpenFile(tempDestination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("failed to create destination binary: %w", err)
	}

	if _, err := io.Copy(outFile, inFile); err != nil {
		_ = outFile.Close()
		_ = os.Remove(tempDestination)
		return fmt.Errorf("failed to write destination binary: %w", err)
	}
	if err := outFile.Close(); err != nil {
		_ = os.Remove(tempDestination)
		return fmt.Errorf("failed to finalize destination binary: %w", err)
	}
	if err := os.Chmod(tempDestination, 0o755); err != nil {
		_ = os.Remove(tempDestination)
		return fmt.Errorf("failed to set destination mode: %w", err)
	}
	if err := os.Rename(tempDestination, destination); err != nil {
		_ = os.Remove(tempDestination)
		return fmt.Errorf("failed to install picoclaw binary: %w", err)
	}
	return nil
}
