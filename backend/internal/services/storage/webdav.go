package storage

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/studio-b12/gowebdav"
)

// 确保 WebDAVStorage 实现 Driver 接口
var _ Driver = (*WebDAVStorage)(nil)

type WebDAVStorage struct {
	client   *gowebdav.Client
	basePath string
}

type WebDAVConfig struct {
	URL      string
	Username string
	Password string
	BasePath string
}

func NewWebDAVStorage(cfg WebDAVConfig) (*WebDAVStorage, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, fmt.Errorf("WebDAV URL 不能为空")
	}

	client := gowebdav.NewClient(cfg.URL, cfg.Username, cfg.Password)
	basePath := strings.TrimSuffix(strings.TrimSpace(cfg.BasePath), "/")
	if basePath == "" {
		basePath = "/releasehub"
	}

	// 确保基础目录存在
	if err := client.MkdirAll(basePath, 0o755); err != nil {
		return nil, fmt.Errorf("创建 WebDAV 基础目录失败: %w", err)
	}

	return &WebDAVStorage{
		client:   client,
		basePath: basePath,
	}, nil
}

func (s *WebDAVStorage) Put(ctx context.Context, objectPath string, reader io.Reader) (*StoredObject, error) {
	remotePath := s.remotePath(objectPath)
	dir := filepath.Dir(remotePath)

	// 确保目录存在
	if err := s.client.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建 WebDAV 目录失败: %w", err)
	}

	// 读取全部内容（WebDAV 客户端不支持流式写入）
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("读取上传数据失败: %w", err)
	}

	if err := s.client.Write(remotePath, data, 0o644); err != nil {
		return nil, fmt.Errorf("上传到 WebDAV 失败: %w", err)
	}

	return &StoredObject{
		Path:     filepath.ToSlash(filepath.Clean(objectPath)),
		AbsPath:  remotePath,
		Size:     int64(len(data)),
		Filename: filepath.Base(remotePath),
	}, nil
}

func (s *WebDAVStorage) Open(ctx context.Context, objectPath string) (io.ReadCloser, *StoredObject, error) {
	remotePath := s.remotePath(objectPath)

	data, err := s.client.Read(remotePath)
	if err != nil {
		return nil, nil, fmt.Errorf("从 WebDAV 读取失败: %w", err)
	}

	reader := io.NopCloser(strings.NewReader(string(data)))
	return reader, &StoredObject{
		Path:     filepath.ToSlash(filepath.Clean(objectPath)),
		AbsPath:  remotePath,
		Size:     int64(len(data)),
		Filename: filepath.Base(remotePath),
	}, nil
}

func (s *WebDAVStorage) Delete(ctx context.Context, objectPath string) error {
	remotePath := s.remotePath(objectPath)

	if err := s.client.Remove(remotePath); err != nil {
		return fmt.Errorf("从 WebDAV 删除失败: %w", err)
	}

	return nil
}

func (s *WebDAVStorage) SetLatestTag(ctx context.Context, provider string, owner string, repo string, tag string) error {
	manifestPath := filepath.ToSlash(filepath.Join(
		s.basePath,
		safeSegment(provider),
		safeSegment(owner),
		safeSegment(repo),
		"latest.json",
	))

	// WebDAV 不支持符号链接，只写 latest.json
	manifest := LatestManifest{
		Tag:       tag,
		UpdatedAt: timeNowUTC(),
	}
	data := []byte(fmt.Sprintf(`{"tag":"%s","updatedAt":"%s"}`, manifest.Tag, manifest.UpdatedAt))

	dir := filepath.Dir(manifestPath)
	if err := s.client.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建 WebDAV 目录失败: %w", err)
	}

	if err := s.client.Write(manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("写入 latest.json 失败: %w", err)
	}

	return nil
}

// Capabilities 声明 WebDAV 存储不支持符号链接
func (s *WebDAVStorage) Capabilities() Capabilities {
	return Capabilities{CanSymlink: false}
}

func (s *WebDAVStorage) remotePath(objectPath string) string {
	cleanPath := filepath.ToSlash(filepath.Clean(objectPath))
	return filepath.ToSlash(filepath.Join(s.basePath, cleanPath))
}
