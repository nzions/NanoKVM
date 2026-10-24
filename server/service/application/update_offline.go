package application

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"NanoKVM-Server/proto"
	"NanoKVM-Server/service/updatesign"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

var validFilenameRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func (s *Service) OfflineUpdate(c *gin.Context) {
	var rsp proto.Response

	if !acquireUpdateLock() {
		rsp.ErrRsp(c, -1, "update already in progress")
		return
	}

	if err := offlineUpdate(c); err != nil {
		releaseUpdateLock()
		rsp.ErrRsp(c, -1, fmt.Sprintf("update failed: %s", err))
		return
	}

	rsp.OkRsp(c)
	log.Debugf("offline update application success")

	go restartServices()
}

func offlineUpdate(c *gin.Context) error {
	if err := prepareCacheForUpdate(); err != nil {
		return err
	}
	workspace, err := newUpdateWorkspace(CacheDir)
	if err != nil {
		return err
	}
	defer func() {
		if err := workspace.Close(); err != nil {
			log.Warnf("failed to clean update workspace %s: %v", workspace.dir, err)
		}
	}()
	if err := ensureInstallFilesystem(workspace.dir, AppDir, BackupDir); err != nil {
		return err
	}

	maxRequestBytes := int64(maxPackageSize) + (1 << 20)
	contentLength := c.Request.ContentLength
	if contentLength > maxRequestBytes {
		return fmt.Errorf("offline update request exceeds %d bytes", maxRequestBytes)
	}
	if contentLength > 0 {
		if err := ensureFreeSpace(workspace.dir, uint64(contentLength)); err != nil {
			return err
		}
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBytes)

	if err := createSentinelFile(); err != nil {
		return err
	}
	defer removeSentinelFile()

	reader, err := c.Request.MultipartReader()
	if err != nil {
		log.Errorf("Invalid multipart data: %v", err)
		return fmt.Errorf("invalid multipart data: %w", err)
	}

	target, envelope, err := processUpload(reader, contentLength, workspace.dir)
	if err != nil {
		log.Errorf("failed to upload install package: %v", err)
		return err
	}

	// The version is the one the package file name declares (validateFilename
	// pins it to nanokvm_X.Y.Z.tar.gz). Nothing is inspected, extracted or
	// installed until the signature over that name and the file's sha256 passes.
	archiveName := filepath.Base(target)
	expectedRoot := strings.TrimSuffix(archiveName, ".tar.gz")
	expectedVersion := strings.TrimPrefix(expectedRoot, "nanokvm_")
	sum, err := fileSHA256(target)
	if err != nil {
		return err
	}
	signer, err := updatesign.Verify(updatesign.AppName(expectedVersion), envelope, sum)
	if err != nil {
		log.Errorf("offline update signature check failed: %v", err)
		return err
	}
	log.Infof("offline update %s signed by %s", expectedVersion, signer)

	info, err := inspectUpdateArchive(target, expectedRoot)
	if err != nil {
		return fmt.Errorf("inspect update package: %w", err)
	}
	if err := ensureExpandedSpace(workspace.dir, info.expandedBytes); err != nil {
		return err
	}
	sourceDir, err := extractUpdateArchive(target, workspace.dir, expectedRoot)
	if err != nil {
		return fmt.Errorf("extract update package: %w", err)
	}
	if err := validateExtractedPackage(sourceDir, expectedVersion); err != nil {
		return err
	}
	if err := installPreparedPackage(sourceDir); err != nil {
		log.Errorf("failed to install package: %v", err)
		return err
	}

	return nil
}

func createSentinelFile() error {
	file, err := os.OpenFile(
		sentinelPath,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		sentinelPermission,
	)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("download already in progress")
		}
		log.Errorf("Failed to create sentinel file: %v", err)
		return fmt.Errorf("failed to create sentinel file: %w", err)
	}

	if _, err := file.WriteString("downloading"); err != nil {
		_ = file.Close()
		_ = os.Remove(sentinelPath)
		return fmt.Errorf("failed to initialize sentinel file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(sentinelPath)
		return fmt.Errorf("failed to close sentinel file: %w", err)
	}

	return nil
}

// processUpload reads the package ("file") and its signature envelope
// ("signature", the .sig file) from the multipart body, in either order.
func processUpload(reader *multipart.Reader, contentLength int64, workspaceDir string) (string, []byte, error) {
	var outPath string
	var envelope []byte

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, fmt.Errorf("failed to read multipart: %w", err)
		}

		switch part.FormName() {
		case "file":
			if outPath != "" {
				return "", nil, fmt.Errorf("multiple files uploaded")
			}
			outPath, err = saveUploadedFile(part, contentLength, workspaceDir)
			if err != nil {
				return "", nil, err
			}
		case "signature":
			if envelope != nil {
				return "", nil, fmt.Errorf("multiple signatures uploaded")
			}
			envelope, err = updatesign.ReadEnvelope(part)
			if err != nil {
				return "", nil, err
			}
		}
	}

	if outPath == "" {
		return "", nil, fmt.Errorf("no file uploaded")
	}
	if envelope == nil {
		return "", nil, fmt.Errorf("no signature uploaded: select the .sig file that belongs to the package")
	}

	return outPath, envelope, nil
}

func saveUploadedFile(part *multipart.Part, contentLength int64, workspaceDir string) (string, error) {
	filename := part.FileName()
	if filename == "" {
		return "", fmt.Errorf("no filename provided")
	}

	if err := validateFilename(filename); err != nil {
		return "", err
	}

	outPath := filepath.Join(workspaceDir, filename)
	out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	success := false
	defer func() {
		_ = out.Close()
		if !success {
			_ = os.Remove(outPath)
		}
	}()

	if contentLength < 0 {
		contentLength = 0
	}
	pw := newProgressWriter(out, contentLength)
	defer pw.Stop()

	written, err := io.Copy(pw, io.LimitReader(part, int64(maxPackageSize)+1))
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	if written > int64(maxPackageSize) {
		return "", fmt.Errorf("uploaded package exceeds %d bytes", maxPackageSize)
	}
	if err := out.Close(); err != nil {
		return "", fmt.Errorf("failed to close output file: %w", err)
	}
	success = true

	return outPath, nil
}

func validateFilename(filename string) error {
	baseName := filepath.Base(filename)

	// Check if the path contains directory components
	if baseName != filename {
		log.Warnf("Path detected in filename: %s", filename)
		return fmt.Errorf("path detected in filename")
	}

	// Check for path traversal attempts
	if strings.Contains(filename, "..") {
		log.Warnf("Path traversal attempt: %s", filename)
		return fmt.Errorf("invalid filename: path traversal detected")
	}

	// Validate filename characters
	if !validFilenameRegex.MatchString(filename) {
		log.Warnf("Invalid filename characters: %s", filename)
		return fmt.Errorf("invalid filename: contains invalid characters")
	}
	if !packageNamePattern.MatchString(filename) {
		return fmt.Errorf("invalid update package name")
	}

	return nil
}

func removeSentinelFile() {
	_ = os.Remove(sentinelPath)
}
