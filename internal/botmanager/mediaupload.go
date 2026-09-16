package botmanager

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

// mediaHTTPClient is a shared HTTP client for streaming media uploads.
// Uses an optimized transport with connection pooling and dial/response timeouts.
var mediaHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	},
}

// streamingMediaSource implements uploader/source.Source by streaming an HTTP
// response body directly into the Telegram uploader, without reading the whole
// file into memory first.
//
// It validates Content-Type and basic HTML-detection on the response headers
// before letting the uploader start consuming the body.
type streamingMediaSource struct {
	resp *http.Response
	name string
}

func (s *streamingMediaSource) Read(p []byte) (int, error) { return s.resp.Body.Read(p) }
func (s *streamingMediaSource) Close() error               { return s.resp.Body.Close() }
func (s *streamingMediaSource) Name() string               { return s.name }
func (s *streamingMediaSource) Size() int64                { return s.resp.ContentLength }

// openMediaStream opens a streaming HTTP connection to the given URL and returns
// a ReadCloser ready to be piped to the uploader.  It performs header-level
// validation (status code, Content-Type) without buffering the body.
func openMediaStream(ctx context.Context, rawURL string) (*streamingMediaSource, error) {
	var resp *http.Response
	var err error

	for attempt := 0; attempt < 2; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; TelegramBot/1.0)")

		resp, err = mediaHTTPClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		if attempt == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
	}

	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}

	ct := resp.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(ct)
	if mediaType == "text/html" {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("expected media file but got Content-Type text/html from %s", rawURL)
	}

	// Determine a sensible filename from the final URL (after redirects).
	finalURL := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	name := filenameFromURL(finalURL, ct)

	return &streamingMediaSource{resp: resp, name: name}, nil
}

// filenameFromURL derives a filename from the URL path, falling back to a
// Content-Type-based extension when the path has none.
func filenameFromURL(rawURL, contentType string) string {
	u, err := url.Parse(rawURL)
	if err == nil {
		base := path.Base(u.Path)
		// Accept if it looks like a real filename (has a dot and extension ≥ 2 chars)
		if idx := strings.LastIndex(base, "."); idx >= 0 && len(base)-idx >= 3 {
			return base
		}
	}
	// Fallback: guess from Content-Type
	mediaType, _, _ := mime.ParseMediaType(contentType)
	switch mediaType {
	case "image/jpeg":
		return "photo.jpg"
	case "image/png":
		return "photo.png"
	case "image/webp":
		return "photo.webp"
	case "image/gif":
		return "image.gif"
	case "video/mp4":
		return "video.mp4"
	case "video/webm":
		return "video.webm"
	case "audio/mpeg", "audio/mp3":
		return "audio.mp3"
	case "audio/ogg":
		return "audio.ogg"
	default:
		return "file.bin"
	}
}

// uploadFromURL streams a remote URL directly into Telegram via MTProto upload,
// without buffering the full file in memory.
//
// If forceSmall is true the upload is forced through saveFilePart (small upload),
// which is required for InputMediaUploadedPhoto. This is safe only when the
// content is known to be ≤ ~10 MB (Telegram's limit for small uploads).
// For larger files (videos, documents) pass forceSmall=false.
//
// Returns (inputFile, mimeType, filename, error).
// mimeType and filename are derived from the HTTP response headers / URL.
func (b *BotInstance) uploadFromURL(ctx context.Context, rawURL string) (tg.InputFileClass, string, string, error) {
	src, err := openMediaStream(ctx, rawURL)
	if err != nil {
		return nil, "", "", fmt.Errorf("open stream: %w", err)
	}
	defer src.Close()

	u := uploader.NewUploader(b.raw)
	size := src.Size() // -1 if Content-Length unknown

	// gotd: size=-1 → big upload (InputFileBig); size≥0 → auto small/big by threshold.
	// InputMediaUploadedPhoto requires InputFile (small), not InputFileBig.
	// Detect this early and if size is unknown but mime is image, buffer into RAM
	// (images are typically < 20 MB, well within Telegram limits).
	ct := src.resp.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(ct)

	var inputFile tg.InputFileClass
	if size < 0 && strings.HasPrefix(mediaType, "image/") {
		// Unknown Content-Length for an image: buffer to guarantee small upload.
		data, err := io.ReadAll(src)
		if err != nil {
			return nil, "", "", fmt.Errorf("buffer image: %w", err)
		}
		inputFile, err = u.FromBytes(ctx, src.name, data)
		if err != nil {
			return nil, "", "", fmt.Errorf("upload buffered image to telegram: %w", err)
		}
	} else {
		// Size known (or not an image) — pass actual size so gotd picks small vs big correctly.
		inputFile, err = u.Upload(ctx, uploader.NewUpload(src.name, src, size))
		if err != nil {
			return nil, "", "", fmt.Errorf("upload to telegram: %w", err)
		}
	}

	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	return inputFile, mediaType, src.name, nil
}

// isMediaURL returns true if the string looks like an http/https URL.
func isMediaURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// isImage returns true if the MIME type is an image type Telegram supports as photo.
func isImage(mimeType string) bool {
	return strings.HasPrefix(mimeType, "image/") &&
		mimeType != "image/gif" && mimeType != "image/webp"
}

// inputMediaFromReader converts an uploaded inputFile + mimeType into the
// appropriate InputMedia variant (photo vs document).
func inputMediaFromReader(inputFile tg.InputFileClass, mimeType, filename string, spoiler, nosoundVideo bool) tg.InputMediaClass {
	if isImage(mimeType) {
		return &tg.InputMediaUploadedPhoto{
			File:    inputFile,
			Spoiler: spoiler,
		}
	}
	attrs := []tg.DocumentAttributeClass{
		&tg.DocumentAttributeFilename{FileName: filename},
	}
	return &tg.InputMediaUploadedDocument{
		File:         inputFile,
		MimeType:     mimeType,
		Spoiler:      spoiler,
		NosoundVideo: nosoundVideo,
		Attributes:   attrs,
	}
}

// getPhotoCache looks up an InputPhoto in in-memory cache, then Redis.
func (b *BotInstance) getPhotoCache(ctx context.Context, key string) *tg.InputPhoto {
	b.mediaCacheMu.RLock()
	photo, ok := b.mediaCache[key]
	b.mediaCacheMu.RUnlock()
	if ok && photo != nil {
		return photo
	}

	b.mu.RLock()
	botID := b.botID
	b.mu.RUnlock()
	if botID == 0 || b.redisStore == nil {
		return nil
	}

	id, ah, fr, hit := b.redisStore.GetPhotoCache(ctx, botID, key)
	if !hit {
		return nil
	}

	photo = &tg.InputPhoto{
		ID:            id,
		AccessHash:    ah,
		FileReference: fr,
	}

	b.mediaCacheMu.Lock()
	b.mediaCache[key] = photo
	b.mediaCacheMu.Unlock()

	return photo
}

// setPhotoCache updates in-memory cache and asynchronously writes to Redis.
func (b *BotInstance) setPhotoCache(ctx context.Context, key string, photo *tg.InputPhoto) {
	if photo == nil {
		return
	}
	b.mediaCacheMu.Lock()
	b.mediaCache[key] = photo
	b.mediaCacheMu.Unlock()

	b.mu.RLock()
	botID := b.botID
	b.mu.RUnlock()
	if botID != 0 && b.redisStore != nil {
		go func() {
			c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = b.redisStore.SavePhotoCache(c, botID, key, photo.ID, photo.AccessHash, photo.FileReference)
		}()
	}
}

// getDocCache looks up an InputDocument in in-memory cache, then Redis.
func (b *BotInstance) getDocCache(ctx context.Context, key string) *tg.InputDocument {
	b.mediaCacheMu.RLock()
	doc, ok := b.docCache[key]
	b.mediaCacheMu.RUnlock()
	if ok && doc != nil {
		return doc
	}

	b.mu.RLock()
	botID := b.botID
	b.mu.RUnlock()
	if botID == 0 || b.redisStore == nil {
		return nil
	}

	id, ah, fr, hit := b.redisStore.GetDocCache(ctx, botID, key)
	if !hit {
		return nil
	}

	doc = &tg.InputDocument{
		ID:            id,
		AccessHash:    ah,
		FileReference: fr,
	}

	b.mediaCacheMu.Lock()
	b.docCache[key] = doc
	b.mediaCacheMu.Unlock()

	return doc
}

// setDocCache updates in-memory cache and asynchronously writes to Redis.
func (b *BotInstance) setDocCache(ctx context.Context, key string, doc *tg.InputDocument) {
	if doc == nil {
		return
	}
	b.mediaCacheMu.Lock()
	b.docCache[key] = doc
	b.mediaCacheMu.Unlock()

	b.mu.RLock()
	botID := b.botID
	b.mu.RUnlock()
	if botID != 0 && b.redisStore != nil {
		go func() {
			c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = b.redisStore.SaveDocCache(c, botID, key, doc.ID, doc.AccessHash, doc.FileReference)
		}()
	}
}
