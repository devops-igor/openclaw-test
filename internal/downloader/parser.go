package downloader

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/igorkon/youtube-downloader-bot/pkg/logger"
	"github.com/igorkon/youtube-downloader-bot/pkg/types"
)

// Parser parses yt-dlp -F output into structured Format slices.
type Parser struct {
	log *logger.Logger
}

// NewParser creates a new Parser with the given logger.
func NewParser(log *logger.Logger) *Parser {
	return &Parser{log: log.WithComponent("parser")}
}

// ParseFormats parses the output of `yt-dlp -F` into a slice of Format structs.
// It skips header lines and separator lines, extracting structured data from each format row.
func (p *Parser) ParseFormats(output string) ([]types.Format, error) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	var formats []types.Format
	lineNum := 0
	dataStarted := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Skip header lines (before the separator line)
		if !dataStarted {
			if strings.Contains(line, "ID") && strings.Contains(line, "EXT") && strings.Contains(line, "RESOLUTION") {
				continue // header row
			}
			if isSeparatorLine(line) {
				dataStarted = true
				continue
			}
			continue
		}

		// Skip separator lines between sections
		if strings.TrimSpace(line) == "" || isSeparatorLine(line) {
			continue
		}

		// Skip storyboard formats (e.g., sb0, sb1, sb2) — not downloadable
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "sb") {
			continue
		}

		// Parse the format line
		fmt, err := p.parseFormatLine(line)
		if err != nil {
			p.log.Warn("Failed to parse format line", "line_num", lineNum, "line", line, "error", err)
			continue // skip unparseable lines, don't fail the whole parse
		}
		formats = append(formats, fmt)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning yt-dlp output: %w", err)
	}

	p.log.Debug("Parsed formats", "count", len(formats))
	return formats, nil
}

// isSeparatorLine checks if a line is a separator line (dashes or box-drawing chars).
// yt-dlp uses either ASCII dashes (-) or Unicode box-drawing (─) depending on locale/version.
func isSeparatorLine(line string) bool {
	if strings.Contains(line, "─") {
		return true
	}
	dashCount := 0
	totalChars := 0
	for _, r := range line {
		totalChars++
		if r == '-' || r == '─' {
			dashCount++
		}
	}
	// A separator line is mostly dashes (at least 80% or 10+ dashes)
	if totalChars > 0 && dashCount >= 10 {
		return true
	}
	return false
}

// parseFormatLine parses a single format line from yt-dlp -F output.
// Flexible parsing that handles various yt-dlp output formats.
func (p *Parser) parseFormatLine(line string) (types.Format, error) {
	// Strategy: split on the │ or | separator to get left side (ID, EXT, RESOLUTION, FPS)
	// and right side (FILESIZE, TBR, PROTO, codecs...)
	// yt-dlp may use either Unicode box-drawing (│) or ASCII pipe (|)
	parts := strings.SplitN(line, "│", 2)
	if len(parts) != 2 {
		parts = strings.SplitN(line, "|", 2)
	}
	if len(parts) != 2 {
		return types.Format{}, fmt.Errorf("no separator found in line: %s", line)
	}

	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])

	// Parse left side: ID EXT RESOLUTION [FPS]
	leftFields := strings.Fields(left)
	if len(leftFields) < 3 {
		return types.Format{}, fmt.Errorf("insufficient left-side fields: %s", left)
	}

	f := types.Format{
		ID:  leftFields[0],
		Ext: leftFields[1],
	}

	// RESOLUTION may be multi-word (e.g., "audio only")
	if leftFields[2] == "audio" && len(leftFields) > 3 && leftFields[3] == "only" {
		f.Resolution = "audio only"
	} else {
		f.Resolution = leftFields[2]
	}

	// Parse right side: FILESIZE TBR PROTO CODECS...
	// Note: yt-dlp sometimes prefixes filesize with ~ for approximate sizes
	rightFields := strings.Fields(right)
	if len(rightFields) >= 1 {
		f.Filesize = rightFields[0]
		if f.Filesize == "~" && len(rightFields) >= 2 {
			// "~ 20.70MiB" means approximate filesize — take the actual value
			f.Filesize = "~" + rightFields[1]
		}
	}

	// Extract codec info and description from the rest
	codecInfo := p.extractCodecInfo(right)
	f.Codec = codecInfo.Codec
	f.Description = p.buildDescription(f, codecInfo)

	return f, nil
}

// codecInfo holds extracted codec information from the right side of the format line.
type codecInfo struct {
	Codec    string
	VCODEC   string
	ACODEC   string
	MoreInfo string
}

// extractCodecInfo extracts codec details from the right side of a format line.
func (p *Parser) extractCodecInfo(right string) codecInfo {
	info := codecInfo{}
	fields := strings.Fields(right)

	// yt-dlp -F format: FILESIZE TBR PROTO VCODEC VBR ACODEC ABR ASR MORE_INFO
	// After PROTO (index 2), codecs start at index 3+
	// Special cases:
	//   "audio only" format: FILESIZE TBR PROTO audio only <actual_codec> ABR ASR...
	//   "video only" format: FILESIZE TBR PROTO video only
	audioOnlyMarker := false
	videoOnlyMarker := false
	if len(fields) >= 5 {
		for i := 3; i < len(fields); i++ {
			f := fields[i]
			// Skip "only" keyword
			if f == "only" {
				continue
			}
			// "video only" marker
			if f == "video" {
				videoOnlyMarker = true
				continue
			}
			// "audio only" marker
			if f == "audio" {
				audioOnlyMarker = true
				continue
			}
			// Actual codec names
			if strings.Contains(f, "avc") || strings.Contains(f, "av01") || strings.Contains(f, "vp09") || strings.Contains(f, "vp9") {
				info.VCODEC = f
				continue
			}
			if strings.Contains(f, "opus") || strings.Contains(f, "mp4a") || strings.Contains(f, "vorbis") {
				info.ACODEC = f
				continue
			}
		}
	}

	// Set markers only if no specific codec was found
	if videoOnlyMarker && info.VCODEC == "" {
		info.VCODEC = "video only"
	}
	if audioOnlyMarker && info.ACODEC == "" {
		info.ACODEC = "audio only"
	}

	// Build combined codec string
	switch {
	case info.VCODEC != "" && info.ACODEC != "" && info.VCODEC != "video only" && info.ACODEC != "audio only":
		info.Codec = info.VCODEC + "+" + info.ACODEC
	case info.VCODEC != "" && info.VCODEC != "video only":
		info.Codec = info.VCODEC
	case info.ACODEC != "" && info.ACODEC != "audio only":
		info.Codec = info.ACODEC
	case info.ACODEC == "audio only":
		info.Codec = "audio only"
	case info.VCODEC == "video only":
		info.Codec = "video only"
	default:
		info.Codec = "unknown"
	}

	// Extract "more info" portion (after ABR/ASR)
	for i := len(fields) - 1; i >= 0; i-- {
		if strings.Contains(fields[i], ",") {
			info.MoreInfo = strings.Join(fields[i:], " ")
			break
		}
	}

	return info
}

// buildDescription creates a human-readable description of the format.
func (p *Parser) buildDescription(f types.Format, ci codecInfo) string {
	var parts []string

	if f.Resolution == "audio only" {
		parts = append(parts, "audio only")
	} else {
		parts = append(parts, f.Resolution)
	}

	if ci.VCODEC != "" && ci.VCODEC != "video only" {
		parts = append(parts, ci.VCODEC)
	}
	if ci.ACODEC != "" && ci.ACODEC != "audio only" {
		parts = append(parts, ci.ACODEC)
	}

	if ci.MoreInfo != "" {
		parts = append(parts, strings.TrimPrefix(ci.MoreInfo, ", "))
	}

	return strings.Join(parts, " ")
}
