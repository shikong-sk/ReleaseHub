package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// 确保 S3Storage 实现 Driver 接口
var _ Driver = (*S3Storage)(nil)

type S3Storage struct {
	endpoint  string
	bucket    string
	region    string
	accessKey string
	secretKey string
	prefix    string
	client    *http.Client
}

type S3Config struct {
	Endpoint  string
	Bucket    string
	Region    string
	AccessKey string
	SecretKey string
	Prefix    string
}

func NewS3Storage(cfg S3Config) (*S3Storage, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, fmt.Errorf("S3 Endpoint 不能为空")
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("S3 Bucket 不能为空")
	}
	prefix := strings.TrimSuffix(strings.TrimSpace(cfg.Prefix), "/")
	if prefix == "" {
		prefix = "releasehub"
	}

	return &S3Storage{
		endpoint:  strings.TrimSuffix(strings.TrimSpace(cfg.Endpoint), "/"),
		bucket:    strings.TrimSpace(cfg.Bucket),
		region:    strings.TrimSpace(cfg.Region),
		accessKey: strings.TrimSpace(cfg.AccessKey),
		secretKey: strings.TrimSpace(cfg.SecretKey),
		prefix:    prefix,
		client:    &http.Client{Timeout: 30 * time.Minute},
	}, nil
}

func (s *S3Storage) Put(ctx context.Context, objectPath string, reader io.Reader) (*StoredObject, error) {
	// 读取全部内容（简单实现，v0.4 可改为分片上传）
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("读取上传数据失败: %w", err)
	}

	key := s.objectKey(objectPath)
	reqURL := s.objectURL(key)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	s.setAuth(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("上传到 S3 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("S3 上传响应异常: HTTP %d", resp.StatusCode)
	}

	return &StoredObject{
		Path:     filepath.ToSlash(filepath.Clean(objectPath)),
		AbsPath:  key,
		Size:     int64(len(data)),
		Filename: filepath.Base(objectPath),
	}, nil
}

func (s *S3Storage) Open(ctx context.Context, objectPath string) (io.ReadCloser, *StoredObject, error) {
	key := s.objectKey(objectPath)
	reqURL := s.objectURL(key)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	s.setAuth(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("从 S3 读取失败: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, nil, fmt.Errorf("S3 读取响应异常: HTTP %d", resp.StatusCode)
	}

	return resp.Body, &StoredObject{
		Path:     filepath.ToSlash(filepath.Clean(objectPath)),
		AbsPath:  key,
		Size:     resp.ContentLength,
		Filename: filepath.Base(objectPath),
	}, nil
}

func (s *S3Storage) Delete(ctx context.Context, objectPath string) error {
	key := s.objectKey(objectPath)
	reqURL := s.objectURL(key)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return err
	}
	s.setAuth(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("从 S3 删除失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("S3 删除响应异常: HTTP %d", resp.StatusCode)
	}

	return nil
}

func (s *S3Storage) SetLatestTag(ctx context.Context, provider string, owner string, repo string, tag string) error {
	// S3 不支持符号链接，写 latest.json
	manifestKey := filepath.ToSlash(filepath.Join(
		s.prefix,
		safeSegment(provider),
		safeSegment(owner),
		safeSegment(repo),
		"latest.json",
	))

	data := []byte(fmt.Sprintf(`{"tag":"%s","updatedAt":"%s"}`, tag, timeNowUTC()))
	reqURL := s.objectURL(manifestKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	s.setAuth(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("写入 S3 latest.json 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("S3 写入 latest.json 响应异常: HTTP %d", resp.StatusCode)
	}

	return nil
}

// Capabilities 声明 S3 存储不支持符号链接
func (s *S3Storage) Capabilities() Capabilities {
	return Capabilities{CanSymlink: false}
}

func (s *S3Storage) objectKey(objectPath string) string {
	cleanPath := filepath.ToSlash(filepath.Clean(objectPath))
	return filepath.ToSlash(filepath.Join(s.prefix, cleanPath))
}

func (s *S3Storage) objectURL(key string) string {
	return fmt.Sprintf("%s/%s/%s", s.endpoint, s.bucket, key)
}

func (s *S3Storage) setAuth(req *http.Request) {
	if s.accessKey != "" {
		// 简单的 Basic Auth 或路径风格鉴权
		// 生产环境需要 AWS V4 签名，v0.3 会引入 AWS SDK
		req.SetBasicAuth(s.accessKey, s.secretKey)
	}
}

// TestConnection 测试 S3 连接是否可用
func (s *S3Storage) TestConnection(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/%s", s.endpoint, s.bucket)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, reqURL, nil)
	if err != nil {
		return err
	}
	s.setAuth(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("S3 连接失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("S3 认证失败: HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("S3 服务不可用: HTTP %d", resp.StatusCode)
	}

	return nil
}

// TestConnection 测试 WebDAV 连接是否可用
func (s *WebDAVStorage) TestConnection(ctx context.Context) error {
	_, err := s.client.ReadDir(s.basePath)
	if err != nil {
		return fmt.Errorf("WebDAV 连接失败: %w", err)
	}
	return nil
}

// buildObjectPath 根据 provider/owner/repo/tag 构建存储路径
// 此函数是公开的，供外部包使用
func BuildObjectPath(provider, owner, repo, tag, filename string) string {
	return filepath.ToSlash(filepath.Join(
		safeSegment(provider),
		safeSegment(owner),
		safeSegment(repo),
		safeSegment(tag),
		filepath.Base(filename),
	))
}

// parseStorageURL 解析存储 URL（用于 S3 endpoint 解析）
func parseStorageURL(rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}
