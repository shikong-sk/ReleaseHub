package downloader

import (
	"io"
	"time"
)

// RateLimitedWriter 限速 io.Writer，maxBytes=0 时不限速
type RateLimitedWriter struct {
	writer   io.Writer
	maxBytes int64
}

func NewRateLimitedWriter(w io.Writer, maxBytesPerSec int64) *RateLimitedWriter {
	return &RateLimitedWriter{writer: w, maxBytes: maxBytesPerSec}
}

func (r *RateLimitedWriter) Write(p []byte) (int, error) {
	if r.maxBytes <= 0 {
		return r.writer.Write(p)
	}

	written := 0
	for written < len(p) {
		// 每次最多写 maxBytes 字节（1秒配额）
		chunk := int64(len(p) - written)
		if chunk > r.maxBytes {
			chunk = r.maxBytes
		}

		start := time.Now()
		n, err := r.writer.Write(p[written : written+int(chunk)])
		written += n
		if err != nil {
			return written, err
		}

		// 等待剩余时间以匀速
		elapsed := time.Since(start)
		if elapsed < time.Second {
			time.Sleep(time.Second - elapsed)
		}
	}
	return written, nil
}
