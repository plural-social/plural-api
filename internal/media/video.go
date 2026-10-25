package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
)

// ErrInvalidVideo is a permanent failure: retrying will not help. The wrapped
// message is shown to the user, so keep it short and plain.
var ErrInvalidVideo = errors.New("invalid video")

// InvalidVideoError carries a user-facing reason and matches ErrInvalidVideo
// with errors.Is.
type InvalidVideoError struct{ Reason string }

func (e InvalidVideoError) Error() string { return e.Reason }

func (e InvalidVideoError) Is(target error) bool { return target == ErrInvalidVideo }

// stderrTailBytes is how much ffmpeg output an error keeps. The end of the
// output is where ffmpeg explains what went wrong.
const stderrTailBytes = 2000

// Tools runs ffprobe and ffmpeg. Paths may be bare names found in PATH.
type Tools struct {
	FFmpegPath  string
	FFprobePath string
}

type ProbeResult struct {
	DurationMS int
	Width      int // after rotation
	Height     int // after rotation
	HasAudio   bool
}

// ffprobeOutput is the part of `ffprobe -print_format json` this package reads.
type ffprobeOutput struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		Tags      struct {
			Rotate string `json:"rotate"` // older files
		} `json:"tags"`
		SideDataList []struct {
			Rotation float64 `json:"rotation"` // newer files
		} `json:"side_data_list"`
	} `json:"streams"`
}

var errUnreadable = InvalidVideoError{Reason: "this file isn't a video we can read"}

// Probe runs ffprobe and returns InvalidVideoError when the file has no video
// stream or cannot be parsed.
func (t Tools) Probe(ctx context.Context, path string) (ProbeResult, error) {
	stdout, err := run(ctx, t.FFprobePath, "-v", "error", "-print_format", "json", "-show_format", "-show_streams", path)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return ProbeResult{}, errUnreadable
		}
		return ProbeResult{}, err
	}

	var out ffprobeOutput
	if err := json.Unmarshal(stdout, &out); err != nil {
		return ProbeResult{}, errUnreadable
	}

	var result ProbeResult
	foundVideo := false
	for _, stream := range out.Streams {
		switch stream.CodecType {
		case "audio":
			result.HasAudio = true
		case "video":
			if foundVideo {
				continue
			}
			foundVideo = true
			result.Width, result.Height = stream.Width, stream.Height
			rotation, _ := strconv.ParseFloat(stream.Tags.Rotate, 64)
			for _, sideData := range stream.SideDataList {
				if sideData.Rotation != 0 {
					rotation = sideData.Rotation
				}
			}
			// Phones record portrait video as landscape plus a rotation flag.
			if quarterTurns := int(math.Round(rotation/90)) % 2; quarterTurns != 0 {
				result.Width, result.Height = result.Height, result.Width
			}
		}
	}
	if !foundVideo {
		return ProbeResult{}, errUnreadable
	}

	seconds, err := strconv.ParseFloat(out.Format.Duration, 64)
	if err != nil {
		return ProbeResult{}, errUnreadable
	}
	result.DurationMS = int(math.Round(seconds * 1000))
	return result, nil
}

// ValidateProbe returns InvalidVideoError for durations outside 1-180 s or
// sizes above 7680 px.
func ValidateProbe(p ProbeResult) error {
	switch {
	case p.DurationMS < VideoMinDurationMS:
		return InvalidVideoError{Reason: "video must be at least 1 second long"}
	case p.DurationMS > VideoMaxDurationMS+videoDurationToleranceMS:
		return InvalidVideoError{Reason: "video must be 3 minutes or shorter"}
	case p.Width <= 0 || p.Height <= 0 || p.Width > videoMaxSide || p.Height > videoMaxSide:
		return InvalidVideoError{Reason: "video resolution isn't supported"}
	}
	return nil
}

// Transcode writes an H.264/AAC MP4 to outPath: short side at most 720 px,
// even sizes, at most 60 fps, index at the start, no metadata.
func (t Tools) Transcode(ctx context.Context, inPath, outPath string) error {
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", inPath,
		// Audio is optional ("?"), so silent videos work.
		"-map", "0:v:0", "-map", "0:a:0?",
		// Short side at most 720, never upscaled, both sides even (yuv420p
		// needs it). ffmpeg applies rotation before filters, so iw/ih are upright.
		"-vf", "scale='if(lt(iw,ih),trunc(min(720,iw)/2)*2,-2)':'if(lt(iw,ih),-2,trunc(min(720,ih)/2)*2)'",
		"-fpsmax", "60",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23",
		"-profile:v", "high", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k", "-ac", "2",
		// Index at the start, so playback begins before the download ends.
		"-movflags", "+faststart",
		// Drops location and device metadata.
		"-map_metadata", "-1",
		"-t", "180",
		outPath,
	}
	_, err := run(ctx, t.FFmpegPath, args...)
	return err
}

// Poster writes a JPEG frame from videoPath to outPath, taken at
// min(0.5 s, duration/2).
func (t Tools) Poster(ctx context.Context, videoPath string, durationMS int, outPath string) error {
	seconds := min(0.5, float64(durationMS)/2000)
	_, err := run(ctx, t.FFmpegPath,
		"-hide_banner", "-loglevel", "error", "-y",
		"-ss", fmt.Sprintf("%.3f", seconds),
		"-i", videoPath,
		"-frames:v", "1", "-q:v", "3",
		outPath,
	)
	return err
}

// run executes a command without a shell and returns its stdout. Errors keep
// the tail of stderr, which is where ffmpeg explains failures.
func run(ctx context.Context, name string, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		tail := stderr.Bytes()
		if len(tail) > stderrTailBytes {
			tail = tail[len(tail)-stderrTailBytes:]
		}
		return nil, fmt.Errorf("running %s: %w: %s", name, err, tail)
	}
	return stdout.Bytes(), nil
}
