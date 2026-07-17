package storage

import "io"

// countingReader 包装 io.Reader 并记录已读取的字节数。
// 用于流式上传时无法预知 Content-Length 的场景，
// 读取完成后通过 n 字段获取实际传输字节数。
type countingReader struct {
	reader io.Reader
	n      int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	read, err := r.reader.Read(p)
	r.n += int64(read)
	return read, err
}

