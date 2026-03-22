package platform

import "errors"

var (
	ErrConfigNotFound    = errors.New("config file not found; run 'packrat init'")
	ErrConfigInvalid     = errors.New("invalid configuration")
	ErrDaemonRunning     = errors.New("daemon is already running")
	ErrDaemonNotRunning  = errors.New("daemon is not running")
	ErrRcloneNotFound      = errors.New("rclone binary not found; install from https://rclone.org")
	ErrRcloneInstallFailed = errors.New("failed to install rclone automatically")
	ErrRemoteNotFound    = errors.New("configured rclone remote not found")
	ErrRemoteUnreachable = errors.New("cannot reach remote storage")
	ErrLockAcquire       = errors.New("another backup is already running")
	ErrDecryptionFailed  = errors.New("decryption failed; wrong key or corrupted data")
	ErrSnapshotNotFound  = errors.New("snapshot not found")
	ErrIntegrityMismatch = errors.New("file integrity check failed; content has been tampered with or corrupted")
)
