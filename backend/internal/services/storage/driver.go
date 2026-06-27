package storage

import (
	"context"
	"io"
)

// Driver 存储驱动接口，所有存储后端（Local/S3/WebDAV）必须实现
type Driver interface {
	// Put 将数据写入指定对象路径
	Put(ctx context.Context, objectPath string, reader io.Reader) (*StoredObject, error)
	// Open 读取指定对象路径的内容
	Open(ctx context.Context, objectPath string) (io.ReadCloser, *StoredObject, error)
	// Delete 删除指定对象路径
	Delete(ctx context.Context, objectPath string) error
	// SetLatestTag 设置最新版本标记
	SetLatestTag(ctx context.Context, provider string, owner string, repo string, tag string) error
}

// Capabilities 存储驱动能力描述
type Capabilities struct {
	CanSymlink bool // 是否支持符号链接（Local 支持，S3/WebDAV 不支持）
}

// CapabilityGetter 可选接口，驱动可实现此接口声明自身能力
type CapabilityGetter interface {
	Capabilities() Capabilities
}

// ListResult 目录列表中的一条记录
type ListResult struct {
	Path  string // 相对于存储根目录的对象路径（如 github/owner/repo/tag/file.tar.gz）
	Size int64
}

// Lister 可选接口，驱动可实现此接口支持目录列举。
// 不实现此接口的驱动在 reconcile 时只能做单向检测（DB→存储）。
type Lister interface {
	List(ctx context.Context, prefix string) ([]ListResult, error)
}
