package bot

import (
	"bytes"
	"context"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	imageCache     sync.Map
	fallbackImage  []byte
	fallbackLoaded bool
	fallbackMutex  sync.Mutex
	httpClient     = &http.Client{Timeout: 10 * time.Second}
	validMimeTypes = map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/webp": true,
		"image/gif":  true,
	}
)

func InitImageCache() error {
	fallbackMutex.Lock()
	defer fallbackMutex.Unlock()

	if fallbackLoaded {
		return nil
	}

	file, err := os.Open("./static/not-found.png")
	if err != nil {
		return fmt.Errorf("failed to open fallback image: %w", err)
	}
	defer file.Close()

	fallbackImage, err = io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read fallback image: %w", err)
	}

	fallbackLoaded = true
	return nil
}

func GetSafePoster(url string) tgbotapi.RequestFileData {
	//start := time.Now()
	//defer func() {
	//	slog.Debug("Execution time", "duration", time.Since(start))
	//}()

	if url == "" {
		return getFallbackImageReader()
	}

	if cached, ok := imageCache.Load(url); ok {
		switch v := cached.(type) {
		case []byte:
			return tgbotapi.FileBytes{
				Name:  "poster.jpg",
				Bytes: v,
			}
		case error:
			return getFallbackImageReader()
		}
	}

	imgData, contentType, err := downloadAndValidateImage(url)
	if err != nil {
		slog.Warn("Failed to download image", "url", url, "error", err)
		imageCache.Store(url, err)
		return getFallbackImageReader()
	}

	ext := getExtensionFromContentType(contentType)

	imageCache.Store(url, imgData)

	return tgbotapi.FileBytes{
		Name:  "poster" + ext,
		Bytes: imgData,
	}
}

func downloadAndValidateImage(url string) ([]byte, string, error) {
	//start := time.Now()
	//defer func() {
	//	slog.Debug("Execution time", "duration", time.Since(start))
	//}()

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, "", fmt.Errorf("http get failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("invalid status code: %d", resp.StatusCode)
	}

	imgData, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, "", fmt.Errorf("read failed: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(imgData)
	}

	if !validMimeTypes[contentType] {
		return nil, "", fmt.Errorf("invalid content type: %s", contentType)
	}

	if len(imgData) < 512 {
		return nil, "", fmt.Errorf("image too small: %d bytes", len(imgData))
	}

	_, _, err = image.DecodeConfig(bytes.NewReader(imgData))
	if err != nil {
		return nil, "", fmt.Errorf("invalid image format: %w", err)
	}

	return imgData, contentType, nil
}

func getExtensionFromContentType(contentType string) string {
	switch {
	case strings.Contains(contentType, "jpeg"):
		return ".jpg"
	case strings.Contains(contentType, "png"):
		return ".png"
	case strings.Contains(contentType, "gif"):
		return ".gif"
	case strings.Contains(contentType, "webp"):
		return ".webp"
	default:
		return ".jpg"
	}
}

func getFallbackImageReader() tgbotapi.RequestFileData {
	if !fallbackLoaded {
		if err := InitImageCache(); err != nil {
			slog.Error("Failed to load fallback image", "error", err)
			return tgbotapi.FilePath("./static/not-found.png")
		}
	}
	return tgbotapi.FileBytes{
		Name:  "not-found.png",
		Bytes: fallbackImage,
	}
}

func ClearImageCachePeriodically(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			clearCount := 0
			imageCache.Range(func(key, value interface{}) bool {
				imageCache.Delete(key)
				clearCount++
				return true
			})
			slog.Info("Image cache cleared", "count", clearCount)
		case <-ctx.Done():
			return
		}
	}
}
