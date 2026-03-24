// Package types provides shared types used across the application.
package types

// Format represents a single available format from yt-dlp -F output.
type Format struct {
	ID          string // Format ID (e.g., "137", "251")
	Description string // Human-readable description (e.g., "1080p", "audio only")
	Resolution  string // Video resolution (e.g., "1920x1080", "audio only")
	Filesize    string // Estimated filesize as reported by yt-dlp (e.g., "50.2MiB", "N/A")
	Codec       string // Codec info (e.g., "avc1.640028", "opus")
	Ext         string // File extension (e.g., "mp4", "webm")
}

// DownloadResult holds the outcome of a successful download.
type DownloadResult struct {
	FilePath string // Absolute path to the downloaded file
	Filename string // Original filename
	Filesize int64  // Size in bytes
	FormatID string // The format ID that was downloaded
}
