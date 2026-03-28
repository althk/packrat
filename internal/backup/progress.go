package backup

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// LoggingProgress returns a ProgressFunc that logs backup progress via slog,
// throttled to at most one message per interval per group. This is used by the
// daemon/scheduler so the log file shows periodic updates on in-progress groups.
func LoggingProgress(logger *slog.Logger, interval time.Duration) ProgressFunc {
	var mu sync.Mutex
	lastLog := make(map[string]time.Time)

	return func(group, stage string, current, total int, bytes, totalBytes int64) {
		mu.Lock()
		defer mu.Unlock()

		now := time.Now()
		key := group + ":" + stage

		// Always log "done" and first "scanning" immediately; throttle the rest.
		if stage != "done" && stage != "scanning" {
			if last, ok := lastLog[key]; ok && now.Sub(last) < interval {
				return
			}
		}
		lastLog[key] = now

		switch stage {
		case "scanning":
			logger.Info("backup progress", "group", group, "stage", "scanning")
		case "uploading":
			logger.Info("backup progress",
				"group", group,
				"stage", "uploading",
				"files", fmt.Sprintf("%d/%d", current, total),
				"size", fmt.Sprintf("%s/%s", fmtBytes(bytes), fmtBytes(totalBytes)),
			)
		case "done":
			logger.Info("backup progress",
				"group", group,
				"stage", "done",
				"files", total,
				"total_size", fmtBytes(totalBytes),
			)
		}
	}
}

func fmtBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
