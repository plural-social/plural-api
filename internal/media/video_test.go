package media

import (
	"context"
	"image"
	_ "image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/nicheinc/expect"
)

var testTools = Tools{FFmpegPath: "ffmpeg", FFprobePath: "ffprobe"}

// makeVideo writes a synthetic clip with ffmpeg's lavfi sources.
func makeVideo(t *testing.T, path string, width, height int, seconds float64, withAudio bool, extraOutputArgs ...string) {
	t.Helper()
	duration := strconv.FormatFloat(seconds, 'f', -1, 64)
	args := []string{
		"-loglevel", "error", "-y", "-f", "lavfi", "-i",
		"testsrc2=size=" + strconv.Itoa(width) + "x" + strconv.Itoa(height) + ":rate=30:duration=" + duration,
	}
	if withAudio {
		args = append(args, "-f", "lavfi", "-i", "sine=frequency=440:duration="+duration, "-c:a", "aac", "-shortest")
	}
	args = append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p")
	args = append(args, extraOutputArgs...)
	args = append(args, path)
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("making test video: %v: %s", err, out)
	}
}

// transcodeAndProbe transcodes in and returns the probe of the output.
func transcodeAndProbe(t *testing.T, in string) ProbeResult {
	t.Helper()
	ctx := context.Background()
	out := filepath.Join(t.TempDir(), "out.mp4")
	expect.ErrorNil(t, testTools.Transcode(ctx, in, out))
	result, err := testTools.Probe(ctx, out)
	expect.ErrorNil(t, err)
	return result
}

func Test_Tools(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
	ctx := context.Background()
	dir := t.TempDir()

	t.Run("Garbage", func(t *testing.T) {
		path := filepath.Join(dir, "garbage.mp4")
		expect.ErrorNil(t, os.WriteFile(path, []byte("not a video"), 0o600))
		_, err := testTools.Probe(ctx, path)
		expect.ErrorIs(ErrInvalidVideo)(t, err)
	})
	t.Run("PortraitWithAudio", func(t *testing.T) {
		in := filepath.Join(dir, "portrait.mp4")
		makeVideo(t, in, 1080, 1920, 3, true)
		out := transcodeAndProbe(t, in)
		expect.Equal(t, [2]int{out.Width, out.Height}, [2]int{720, 1280})
		expect.Equal(t, out.HasAudio, true)
		expect.Equal(t, out.DurationMS > 2850 && out.DurationMS < 3150, true)
	})
	t.Run("LandscapeNoAudio", func(t *testing.T) {
		in := filepath.Join(dir, "landscape.mp4")
		makeVideo(t, in, 1920, 1080, 2, false)
		out := transcodeAndProbe(t, in)
		expect.Equal(t, [2]int{out.Width, out.Height}, [2]int{1280, 720})
		expect.Equal(t, out.HasAudio, false)
	})
	t.Run("OddSize", func(t *testing.T) {
		in := filepath.Join(dir, "odd.mkv")
		// libx264 with yuv420p rejects odd sizes, so the source uses yuv444p.
		args := []string{
			"-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=721x1283:rate=30:duration=1",
			"-c:v", "libx264", "-pix_fmt", "yuv444p", in,
		}
		if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
			t.Fatalf("making odd-size video: %v: %s", err, out)
		}
		out := transcodeAndProbe(t, in)
		expect.Equal(t, out.Width%2, 0)
		expect.Equal(t, out.Height%2, 0)
	})
	t.Run("Rotated", func(t *testing.T) {
		plain := filepath.Join(dir, "plain.mp4")
		makeVideo(t, plain, 1920, 1080, 1, false)
		rotated := filepath.Join(dir, "rotated.mp4")
		// ffmpeg 7+ sets the display matrix with -display_rotation on the input;
		// older versions use the "rotate" stream tag.
		newArgs := []string{"-loglevel", "error", "-y", "-display_rotation", "90", "-i", plain, "-c", "copy", rotated}
		if err := exec.Command("ffmpeg", newArgs...).Run(); err != nil {
			oldArgs := []string{"-loglevel", "error", "-y", "-i", plain, "-c", "copy", "-metadata:s:v", "rotate=90", rotated}
			if out, err := exec.Command("ffmpeg", oldArgs...).CombinedOutput(); err != nil {
				t.Fatalf("making rotated video: %v: %s", err, out)
			}
		}
		probe, err := testTools.Probe(ctx, rotated)
		expect.ErrorNil(t, err)
		expect.Equal(t, [2]int{probe.Width, probe.Height}, [2]int{1080, 1920})
		out := transcodeAndProbe(t, rotated)
		expect.Equal(t, [2]int{out.Width, out.Height}, [2]int{720, 1280})
	})
	t.Run("SmallNotUpscaled", func(t *testing.T) {
		in := filepath.Join(dir, "small.mp4")
		makeVideo(t, in, 320, 568, 1, false)
		out := transcodeAndProbe(t, in)
		expect.Equal(t, [2]int{out.Width, out.Height}, [2]int{320, 568})
	})
	t.Run("Poster", func(t *testing.T) {
		in := filepath.Join(dir, "poster-source.mp4")
		makeVideo(t, in, 640, 360, 2, false)
		poster := filepath.Join(dir, "poster.jpg")
		expect.ErrorNil(t, testTools.Poster(ctx, in, 2000, poster))
		file, err := os.Open(poster)
		expect.ErrorNil(t, err)
		defer file.Close()
		config, format, err := image.DecodeConfig(file)
		expect.ErrorNil(t, err)
		expect.Equal(t, format, "jpeg")
		expect.Equal(t, [2]int{config.Width, config.Height}, [2]int{640, 360})
	})
}

func Test_ValidateProbe(t *testing.T) {
	type testCase struct {
		probe          ProbeResult
		expectedReason string
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			err := ValidateProbe(testCase.probe)
			if testCase.expectedReason == "" {
				expect.ErrorNil(t, err)
				return
			}
			expect.ErrorIs(ErrInvalidVideo)(t, err)
			expect.Equal(t, err.Error(), testCase.expectedReason)
		})
	}

	valid := ProbeResult{DurationMS: 5000, Width: 1080, Height: 1920}
	with := func(change func(*ProbeResult)) ProbeResult {
		p := valid
		change(&p)
		return p
	}
	run("TooShort", testCase{probe: with(func(p *ProbeResult) { p.DurationMS = 999 }), expectedReason: "video must be at least 1 second long"})
	run("TooLong", testCase{probe: with(func(p *ProbeResult) { p.DurationMS = 180_501 }), expectedReason: "video must be 3 minutes or shorter"})
	run("TooWide", testCase{probe: with(func(p *ProbeResult) { p.Width = 7681 }), expectedReason: "video resolution isn't supported"})
	run("ZeroSize", testCase{probe: with(func(p *ProbeResult) { p.Height = 0 }), expectedReason: "video resolution isn't supported"})
	run("Valid", testCase{probe: valid})
	run("Tolerance", testCase{probe: with(func(p *ProbeResult) { p.DurationMS = 180_400 })})
}
