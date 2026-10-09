package botmanager

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"time"
)

const (
	// maxParallelDownloads bounds concurrent /file downloads of one bot.
	maxParallelDownloads = 8
	// downloadSlotWait bounds how long a download waits for a free slot.
	downloadSlotWait = 30 * time.Second
)

// downloadIdleTimeout aborts a download that wrote nothing for this long: a stuck
// upload.getFile must not hold the request (and a download slot) for minutes.
// Overridden in tests.
var downloadIdleTimeout = 60 * time.Second

var (
	// ErrTooManyDownloads means all download slots of the bot stayed busy.
	ErrTooManyDownloads = errors.New("too many concurrent file downloads, retry later")
	// ErrDownloadStalled means Telegram stopped sending file parts.
	ErrDownloadStalled = errors.New("file download stalled")
)

func (b *BotInstance) acquireDownloadSlot(ctx context.Context) (func(), error) {
	b.downloadsOnce.Do(func() { b.downloads = make(chan struct{}, maxParallelDownloads) })
	timer := time.NewTimer(downloadSlotWait)
	defer timer.Stop()
	select {
	case b.downloads <- struct{}{}:
		return func() { <-b.downloads }, nil
	case <-timer.C:
		return nil, ErrTooManyDownloads
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// progressWriter remembers when data was last written.
type progressWriter struct {
	w    io.Writer
	last atomic.Int64 // UnixNano
}

func (p *progressWriter) Write(data []byte) (int, error) {
	n, err := p.w.Write(data)
	p.last.Store(time.Now().UnixNano())
	return n, err
}

// watchDownloadProgress cancels ctx with ErrDownloadStalled when pw makes no progress
// for downloadIdleTimeout. The returned function stops watching.
func watchDownloadProgress(ctx context.Context, pw *progressWriter, cancel context.CancelCauseFunc) func() {
	pw.last.Store(time.Now().UnixNano())
	done := make(chan struct{})
	idle := downloadIdleTimeout
	go func() {
		ticker := time.NewTicker(idle / 4)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if now.Sub(time.Unix(0, pw.last.Load())) >= idle {
					cancel(ErrDownloadStalled)
					return
				}
			}
		}
	}()
	return func() { close(done) }
}
