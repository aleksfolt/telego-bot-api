// Package diag records diagnostics for stalls that are hard to catch by hand: when an
// operation runs too long it logs a warning and, for long stalls, saves a dump of all
// goroutines so the blocking point can be found afterwards.
package diag

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

var (
	// SlowThreshold marks an operation as slow and logs a warning.
	SlowThreshold = 5 * time.Second
	// StallThreshold marks an operation as stalled and triggers a goroutine dump.
	StallThreshold = 30 * time.Second

	dumpInterval = 10 * time.Minute
)

const (
	maxDumps   = 20
	dumpPrefix = "stall-"
)

var (
	mu       sync.RWMutex
	dumpDir  string
	logger   = zap.NewNop()
	lastDump atomic.Int64
)

// Configure sets where goroutine dumps are written. An empty dir disables dumps.
func Configure(dir string, log *zap.Logger) {
	mu.Lock()
	defer mu.Unlock()
	dumpDir = dir
	if log != nil {
		logger = log
	}
}

func current() (string, *zap.Logger) {
	mu.RLock()
	defer mu.RUnlock()
	return dumpDir, logger
}

// Watch starts watching an operation. Call the returned function when it finishes.
// If the operation is still running after StallThreshold, a goroutine dump is saved
// while it is stuck; on completion an operation slower than SlowThreshold is logged.
func Watch(kind, operation string, fields ...zap.Field) func() {
	return watch(kind, operation, true, fields)
}

// WatchSlow only logs operations slower than SlowThreshold. It is meant for operations
// that may legitimately run long (uploads of large files), where a dump would mislead.
func WatchSlow(kind, operation string, fields ...zap.Field) func() {
	return watch(kind, operation, false, fields)
}

func watch(kind, operation string, dump bool, fields []zap.Field) func() {
	start := time.Now()
	timer := time.AfterFunc(StallThreshold, func() {
		if !dump {
			return
		}
		_, log := current()
		all := append([]zap.Field{zap.String("kind", kind), zap.String("operation", operation),
			zap.Duration("running", time.Since(start))}, fields...)
		log.Error("Operation stalled", all...)
		Dump(kind+" "+operation, fields...)
	})
	return func() {
		timer.Stop()
		if elapsed := time.Since(start); elapsed >= SlowThreshold {
			_, log := current()
			all := append([]zap.Field{zap.String("kind", kind), zap.String("operation", operation),
				zap.Duration("duration", elapsed)}, fields...)
			log.Warn("Slow operation", all...)
		}
	}
}

// Dump saves a dump of all goroutines, at most once per dumpInterval, and keeps only
// the newest maxDumps files.
func Dump(reason string, fields ...zap.Field) {
	dir, log := current()
	if dir == "" {
		return
	}
	now := time.Now()
	last := lastDump.Load()
	if last != 0 && now.Sub(time.Unix(0, last)) < dumpInterval {
		return
	}
	if !lastDump.CompareAndSwap(last, now.UnixNano()) {
		return
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		log.Warn("Failed to create stall dump directory", zap.String("dir", dir), zap.Error(err))
		return
	}
	path := filepath.Join(dir, dumpPrefix+now.Format("20060102-150405")+".txt")
	if err := writeDump(path, reason, now); err != nil {
		log.Warn("Failed to write stall dump", zap.String("path", path), zap.Error(err))
		return
	}
	log.Error("Saved goroutine dump for a stall", append([]zap.Field{zap.String("path", path),
		zap.String("reason", reason)}, fields...)...)
	pruneDumps(dir)
}

func writeDump(path, reason string, at time.Time) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "telego-bot-api stall dump\ntime: %s\nreason: %s\n\n", at.Format(time.RFC3339), reason); err != nil {
		return err
	}
	return pprof.Lookup("goroutine").WriteTo(f, 2)
}

func pruneDumps(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var dumps []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), dumpPrefix) && strings.HasSuffix(entry.Name(), ".txt") {
			dumps = append(dumps, entry.Name())
		}
	}
	if len(dumps) <= maxDumps {
		return
	}
	// Names embed the timestamp, so lexical order is chronological.
	sort.Strings(dumps)
	for _, name := range dumps[:len(dumps)-maxDumps] {
		_ = os.Remove(filepath.Join(dir, name))
	}
}
