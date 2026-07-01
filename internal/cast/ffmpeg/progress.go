package ffmpeg

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// WatchProgress parses ffmpeg's -progress key=value stream from r and calls
// fn with the encoder's output position in seconds after each progress
// block. It returns when r is exhausted (ffmpeg exited).
//
// Only out_time_us is trusted: out_time_ms famously also contains
// microseconds (long-standing ffmpeg misnomer), and out_time needs string
// parsing for no benefit.
func WatchProgress(r io.Reader, fn func(seconds float64)) {
	scanner := bufio.NewScanner(r)
	var outTimeUs int64
	for scanner.Scan() {
		line := scanner.Text()
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "out_time_us":
			if v, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
				outTimeUs = v
			}
		case "progress":
			// End of one report block — emit the position we accumulated.
			fn(float64(outTimeUs) / 1e6)
		}
	}
}
