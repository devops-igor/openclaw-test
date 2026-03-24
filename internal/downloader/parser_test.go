package downloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/igorkon/youtube-downloader-bot/pkg/logger"
	"github.com/igorkon/youtube-downloader-bot/pkg/types"
)

func loadTestdata(t *testing.T, filename string) string {
	t.Helper()
	path := filepath.Join("testdata", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read testdata %s: %v", filename, err)
	}
	return string(data)
}

func TestParser_ParseFormats_FullOutput(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}) // suppress logs in tests
	p := NewParser(log)

	output := loadTestdata(t, "formats_output.txt")
	formats, err := p.ParseFormats(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(formats) == 0 {
		t.Fatal("expected at least one format, got none")
	}

	t.Logf("Parsed %d formats", len(formats))

	// Verify first parsed format
	found := make(map[string]bool)
	for _, f := range formats {
		found[f.ID] = true
	}

	// Check some expected format IDs from the testdata
	expectedIDs := []string{"18", "137", "251", "248", "140"}
	for _, id := range expectedIDs {
		if !found[id] {
			t.Errorf("expected format ID %s not found", id)
		}
	}
}

func TestParser_ParseFormats_AudioOnly(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"})
	p := NewParser(log)

	output := loadTestdata(t, "formats_output.txt")
	formats, err := p.ParseFormats(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find audio-only format 251
	var audioFormat *types.Format
	for i := range formats {
		if formats[i].ID == "251" {
			audioFormat = &formats[i]
			break
		}
	}

	if audioFormat == nil {
		t.Fatal("format 251 not found")
	}

	if audioFormat.Resolution != "audio only" {
		t.Errorf("expected resolution 'audio only', got %q", audioFormat.Resolution)
	}

	if audioFormat.Ext != "webm" {
		t.Errorf("expected ext 'webm', got %q", audioFormat.Ext)
	}

	if audioFormat.Codec != "opus" {
		t.Errorf("expected codec 'opus', got %q", audioFormat.Codec)
	}
}

func TestParser_ParseFormats_VideoFormat(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"})
	p := NewParser(log)

	output := loadTestdata(t, "formats_output.txt")
	formats, err := p.ParseFormats(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find video format 137 (1080p)
	var videoFormat *types.Format
	for i := range formats {
		if formats[i].ID == "137" {
			videoFormat = &formats[i]
			break
		}
	}

	if videoFormat == nil {
		t.Fatal("format 137 not found")
	}

	if videoFormat.Resolution != "1920x1080" {
		t.Errorf("expected resolution '1920x1080', got %q", videoFormat.Resolution)
	}

	if videoFormat.Ext != "mp4" {
		t.Errorf("expected ext 'mp4', got %q", videoFormat.Ext)
	}

	if videoFormat.Filesize == "" || videoFormat.Filesize == "N/A" {
		t.Errorf("expected non-empty filesize, got %q", videoFormat.Filesize)
	}
}

func TestParser_ParseFormats_MuxedFormat(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"})
	p := NewParser(log)

	output := loadTestdata(t, "formats_output.txt")
	formats, err := p.ParseFormats(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find muxed format 22 (720p with audio)
	var muxedFormat *types.Format
	for i := range formats {
		if formats[i].ID == "22" {
			muxedFormat = &formats[i]
			break
		}
	}

	if muxedFormat == nil {
		t.Fatal("format 22 not found")
	}

	if muxedFormat.Resolution != "1280x720" {
		t.Errorf("expected resolution '1280x720', got %q", muxedFormat.Resolution)
	}
}

func TestParser_ParseFormats_SingleFormat(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"})
	p := NewParser(log)

	output := loadTestdata(t, "single_format.txt")
	formats, err := p.ParseFormats(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(formats) != 1 {
		t.Fatalf("expected 1 format, got %d", len(formats))
	}

	f := formats[0]
	if f.ID != "18" {
		t.Errorf("expected ID '18', got %q", f.ID)
	}
	if f.Resolution != "640x360" {
		t.Errorf("expected resolution '640x360', got %q", f.Resolution)
	}
	if f.Ext != "mp4" {
		t.Errorf("expected ext 'mp4', got %q", f.Ext)
	}
}

func TestParser_ParseFormats_EmptyOutput(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"})
	p := NewParser(log)

	formats, err := p.ParseFormats("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(formats) != 0 {
		t.Errorf("expected 0 formats from empty output, got %d", len(formats))
	}
}

func TestParser_ParseFormats_NoFormatsInOutput(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"})
	p := NewParser(log)

	output := loadTestdata(t, "empty_output.txt")
	formats, err := p.ParseFormats(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(formats) != 0 {
		t.Errorf("expected 0 formats from error output, got %d", len(formats))
	}
}

func TestParser_ParseFormats_MultipleResolutions(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"})
	p := NewParser(log)

	output := loadTestdata(t, "formats_output.txt")
	formats, err := p.ParseFormats(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Count resolutions
	resolutions := make(map[string]int)
	for _, f := range formats {
		resolutions[f.Resolution]++
	}

	// We should have multiple resolution categories
	if len(resolutions) < 3 {
		t.Errorf("expected at least 3 resolution categories, got %d: %v", len(resolutions), resolutions)
	}

	t.Logf("Resolution distribution: %v", resolutions)
}

func TestParser_ParseFormats_CodesPresent(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"})
	p := NewParser(log)

	output := loadTestdata(t, "formats_output.txt")
	formats, err := p.ParseFormats(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, f := range formats {
		if f.Codec == "" {
			t.Errorf("format %s has empty codec", f.ID)
		}
		if f.Description == "" {
			t.Errorf("format %s has empty description", f.ID)
		}
	}
}

func TestParser_ParseFormats_RealOutput(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"})
	p := NewParser(log)

	output := loadTestdata(t, "formats_real.txt")
	formats, err := p.ParseFormats(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(formats) == 0 {
		t.Fatal("expected at least one format, got none (parser bug: separator/dash detection failing)")
	}

	t.Logf("Parsed %d formats from real output", len(formats))

	// Verify storyboard formats are skipped
	for _, f := range formats {
		if strings.HasPrefix(f.ID, "sb") {
			t.Errorf("storyboard format %s should have been skipped", f.ID)
		}
	}

	// Verify ~ filesize handling (format 91 has "~ 20.70MiB")
	var fmt91 *types.Format
	for i := range formats {
		if formats[i].ID == "91" {
			fmt91 = &formats[i]
			break
		}
	}
	if fmt91 != nil && (fmt91.Filesize == "~" || fmt91.Filesize == "") {
		t.Errorf("format 91 filesize should not be just '~', got %q", fmt91.Filesize)
	}

	// Verify expected formats are present
	ids := make(map[string]bool)
	for _, f := range formats {
		ids[f.ID] = true
	}
	for _, id := range []string{"139", "140", "249", "251", "160", "18", "91"} {
		if !ids[id] {
			t.Errorf("expected format ID %s not found", id)
		}
	}
}

func TestParser_ParseFormats_FilesizePresent(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"})
	p := NewParser(log)

	output := loadTestdata(t, "formats_output.txt")
	formats, err := p.ParseFormats(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundFilesize := 0
	for _, f := range formats {
		if f.Filesize != "" && f.Filesize != "N/A" {
			foundFilesize++
		}
	}

	if foundFilesize == 0 {
		t.Error("expected at least some formats to have filesize data")
	}
}
