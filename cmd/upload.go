package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spotify/save-to-spotify/config"
)

// validateMediaFile checks that the file exists, has an allowed extension, and is within size limits.
func validateMediaFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot access file %q: %w", path, err)
	}

	if info.IsDir() {
		return fmt.Errorf("%q is a directory, not a file", path)
	}

	if info.Size() == 0 {
		return fmt.Errorf("file %q is empty (0 bytes)", path)
	}

	ext := strings.ToLower(filepath.Ext(path))
	if !config.AllowedMediaExtensions[ext] {
		return fmt.Errorf("unsupported file extension %q (allowed: .mp3, .m4a, .mp4, .mov, .wav, .ogg)", ext)
	}

	if info.Size() > config.MaxMediaFileSize {
		sizeMB := float64(info.Size()) / (1024 * 1024)
		maxMB := float64(config.MaxMediaFileSize) / (1024 * 1024)
		if maxMB >= 1024 {
			return fmt.Errorf("file too large (%.1f GB) — maximum is %.0f GB", sizeMB/1024, maxMB/1024)
		}
		return fmt.Errorf("file too large (%.0f MB) — maximum is %.0f MB", sizeMB, maxMB)
	}

	return nil
}

// uploadMultipart uploads a file to GCS using the signed part URLs returned by the backend.
// For files that fit in a single part, this is a single PUT. Multiple parts are uploaded sequentially.
func uploadMultipart(parts []multipartUploadURL, contentType, filePath string, fileSize int64) error {
	if len(parts) == 1 {
		return uploadFile(parts[0].SignedURL, contentType, filePath, fileSize)
	}

	// Multi-part: split file evenly across parts
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer f.Close()

	partSize := fileSize / int64(len(parts))
	for i, part := range parts {
		size := partSize
		if i == len(parts)-1 {
			size = fileSize - partSize*int64(i) // last part gets remainder
		}

		req, err := http.NewRequestWithContext(context.Background(), "PUT", part.SignedURL, io.LimitReader(f, size))
		if err != nil {
			return fmt.Errorf("failed to create upload request for part %d: %w", part.PartNumber, err)
		}
		req.Header.Set("Content-Type", contentType)
		req.ContentLength = size

		info("Uploading part %d/%d...\n", i+1, len(parts))
		resp, err := uploadClient.Do(req)
		if err != nil {
			return fmt.Errorf("upload failed for part %d: %w", part.PartNumber, err)
		}
		resp.Body.Close()

		if !isSuccessStatus(resp.StatusCode) {
			return fmt.Errorf("upload failed for part %d (%d)", part.PartNumber, resp.StatusCode)
		}
	}

	info("Upload complete.\n")
	return nil
}

// uploadFile PUTs the file contents to a signed upload URL.
func uploadFile(uploadURL, contentType, filePath string, fileSize int64) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer f.Close()

	filename := filepath.Base(filePath)
	var reader io.Reader = f

	if isTerminal(os.Stderr) && !config.JSONMode() {
		pr := newProgressReader(f, fileSize, filename)
		reader = pr
		defer pr.finish()
	} else {
		info("Uploading %s...\n", filename)
	}

	req, err := http.NewRequestWithContext(context.Background(), "PUT", uploadURL, reader)
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = fileSize

	resp, err := uploadClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	if !isSuccessStatus(resp.StatusCode) {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		return fmt.Errorf("upload failed (%d): %s", resp.StatusCode, string(body))
	}

	if !isTerminal(os.Stderr) && !config.JSONMode() {
		info("Upload complete.\n")
	}

	return nil
}

// mediaContentType returns the MIME content type for a media file path.
func mediaContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".mov":
		return "video/mp4"
	case ".m4a":
		return "audio/mp4"
	case ".wav":
		return "audio/wav"
	case ".ogg":
		return "audio/ogg"
	default: // .mp3 and others
		return "audio/mpeg"
	}
}

// mediaTypeForFile returns the save-to-spotify-service MediaType enum value for a file.
func mediaTypeForFile(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".mov":
		return "EPISODE_VIDEO"
	default:
		return "EPISODE_AUDIO"
	}
}

const maxImageSize = 1 << 20 // 1 MB

var allowedImageExtensions = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
}

var magicBytes = map[string][]byte{
	"image/jpeg": {0xff, 0xd8, 0xff},
	"image/png":  {0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a},
}

type imageUploadResponse struct {
	UploadToken string `json:"upload_token"`
}

// uploadImage uploads a local image file to the backend and returns an upload token.
// Returns "" with no error if imagePath is empty.
func uploadImage(token *config.TokenData, imagePath string) (string, error) {
	if imagePath == "" {
		return "", nil
	}

	ext := strings.ToLower(filepath.Ext(imagePath))
	contentType, ok := allowedImageExtensions[ext]
	if !ok {
		return "", fmt.Errorf("unsupported image extension %q (allowed: .jpg, .jpeg, .png)", ext)
	}

	data, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("cannot read image file: %w", err)
	}

	if len(data) > maxImageSize {
		sizeMB := float64(len(data)) / (1024 * 1024)
		return "", fmt.Errorf("image too large (%.1f MB) — maximum is 1 MB", sizeMB)
	}

	if magic, ok := magicBytes[contentType]; ok && !bytes.HasPrefix(data, magic) {
		return "", fmt.Errorf("file does not appear to be a valid %s image", ext)
	}

	url, err := config.BackendURLPath("images")
	if err != nil {
		return "", fmt.Errorf("failed to build image upload request URL: %w", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to create image upload request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = int64(len(data))

	ab := startActivity("Uploading image")
	resp, err := doAPIRequest(req, token)
	ab.stop(err == nil && resp != nil && isSuccessStatus(resp.StatusCode))
	if err != nil {
		return "", fmt.Errorf("image upload failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("failed to read image upload response: %w", err)
	}

	if !isSuccessStatus(resp.StatusCode) {
		return "", fmt.Errorf("image upload failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var result imageUploadResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse image upload response: %w", err)
	}

	if result.UploadToken == "" {
		return "", fmt.Errorf("image upload succeeded but no token returned")
	}

	return result.UploadToken, nil
}
