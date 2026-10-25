package application

import (
	"fmt"
	"os"
	"sync"
	"syscall"

	"NanoKVM-Server/utils"
	log "github.com/sirupsen/logrus"
)

var (
	mutex      sync.Mutex
	isUpdating bool
)

func acquireUpdateLock() bool {
	mutex.Lock()
	defer mutex.Unlock()

	if isUpdating {
		return false
	}
	isUpdating = true
	return true
}

func releaseUpdateLock() {
	mutex.Lock()
	defer mutex.Unlock()
	isUpdating = false
}

// installPreparedPackage swaps the staged tree in as /kvmapp.
//
// Invariant: at every instant either /kvmapp is a whole tree, or it is missing and
// /kvmapp.new (complete, synced) or /root/old is whole; S95nanokvm's boot recovery
// renames whichever exists back to /kvmapp.
func installPreparedPackage(sourceDir string) error {
	return installTree(sourceDir, AppDir, StagingDir, BackupDir)
}

func installTree(sourceDir, appDir, stagingDir, backupDir string) error {
	// A stale staging dir comes from an earlier, interrupted attempt.
	if err := os.RemoveAll(stagingDir); err != nil {
		return fmt.Errorf("failed to remove stale staging dir: %w", err)
	}

	if err := os.Rename(sourceDir, stagingDir); err != nil {
		return fmt.Errorf("failed to stage update: %w", err)
	}

	// ChmodRecursively sets files only; the staged root becomes /kvmapp itself.
	if err := os.Chmod(stagingDir, 0o755); err != nil {
		return fmt.Errorf("failed to chmod staging dir: %w", err)
	}
	if err := utils.ChmodRecursively(stagingDir, 0o755); err != nil {
		return fmt.Errorf("failed to chmod: %w", err)
	}

	// The staged tree must be complete on disk before the app dir is touched.
	syscall.Sync()

	if err := os.RemoveAll(backupDir); err != nil {
		return fmt.Errorf("failed to remove backup: %w", err)
	}

	if err := os.Rename(appDir, backupDir); err != nil {
		return fmt.Errorf("failed to back up app: %w", err)
	}
	syscall.Sync()

	if err := os.Rename(stagingDir, appDir); err != nil {
		if restoreErr := os.Rename(backupDir, appDir); restoreErr != nil {
			log.Errorf("Failed to restore backup after update failure: %v", restoreErr)
		}
		return fmt.Errorf("failed to move update in place: %w", err)
	}
	syscall.Sync()

	return nil
}
