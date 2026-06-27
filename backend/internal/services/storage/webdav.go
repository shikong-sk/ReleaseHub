package storage

import (
	"errors"
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

	// 尝试确保基础目录存在，忽略已存在或被锁定的错误
	// 某些 WebDAV 服务（如 OpenList/AList）对已存在目录的 MKCOL 会返回 423 Locked
	if err := client.MkdirAll(basePath, 0o755); err != nil {
		if !isMkdirAcceptable(err) {
			return nil, fmt.Errorf("创建 WebDAV 基础目录失败: %w", err)
		}
	}

	return &WebDAVStorage{
		client:   client,
		basePath: basePath,
	}, nil
}

// isMkdirAcceptable 判断 MkdirAll 返回的错误是否可以接受（目录可能已存在或被临时锁定）
// 支持多层嵌套的 os.PathError → StatusError 结构，以及字符串回退匹配
func isMkdirAcceptable(err error) bool {
	if err == nil {
		return true
	}
	// 405 Method Not Allowed: 目录已存在
	// 423 Locked: 资源被锁定（常见于 OpenList/AList）
	if gowebdav.IsErrCode(err, 405) || gowebdav.IsErrCode(err, 423) {
		return true
	}
	// 递归 unwrap：检查嵌套的 PathError 和 StatusError
	inner := errors.Unwrap(err)
	if inner != nil {
		if gowebdav.IsErrCode(inner, 405) || gowebdav.IsErrCode(inner, 423) {
			return true
		}
		// 再 unwrap 一层
		inner2 := errors.Unwrap(inner)
		if inner2 != nil {
			if gowebdav.IsErrCode(inner2, 405) || gowebdav.IsErrCode(inner2, 423) {
				return true
			}
		}
	}
	// 回退：某些 WebDAV 服务可能返回非标准错误包装
	errStr := err.Error()
	return strings.Contains(errStr, "405") || strings.Contains(errStr, "423")
}

// mkdirAllSafe 安全地创建目录，对已存在或被锁定的目录做容错
func (s *WebDAVStorage) mkdirAllSafe(dir string) error {
	if err := s.client.MkdirAll(dir, 0o755); err != nil {
		if isMkdirAcceptable(err) {
			return nil
		}
		return fmt.Errorf("创建 WebDAV 目录失败: %w", err)
	}
	return nil
}

func (s *WebDAVStorage) Put(ctx context.Context, objectPath string, reader io.Reader) (*StoredObject, error) {
	remotePath := s.remotePath(objectPath)
	dir := filepath.Dir(remotePath)

	// 确保目录存在（容错处理）
	if err := s.mkdirAllSafe(dir); err != nil {
		return nil, err
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
	if err := s.mkdirAllSafe(dir); err != nil {
		return err
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

// List 列举指定前缀下的所有文件（递归遍历 WebDAV 目录）
func (s *WebDAVStorage) List(ctx context.Context, prefix string) ([]ListResult, error) {
	var results []ListResult
	searchPath := s.basePath
	if strings.TrimSpace(prefix) != "" {
		searchPath = filepath.ToSlash(filepath.Join(s.basePath, prefix))
	}

	err := s.walkWebDAV(ctx, searchPath, &results)
	if err != nil {
		return results, err
	}
	return results, nil
}

func (s *WebDAVStorage) walkWebDAV(ctx context.Context, dirPath string, results *[]ListResult) error {
	files, err := s.client.ReadDir(dirPath)
	if err != nil {
		return nil // 目录不存在或无权限，跳过
	}

	for _, f := range files {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		remotePath := filepath.ToSlash(filepath.Join(dirPath, f.Name()))

		if f.IsDir() {
			if err := s.walkWebDAV(ctx, remotePath, results); err != nil {
				return err
			}
			continue
		}

		// 跳过元数据文件
		name := f.Name()
		if name == "latest.json" {
			continue
		}

		// 去掉 basePath 前缀，返回相对路径
		relPath := strings.TrimPrefix(remotePath, s.basePath+"/")
		if relPath == remotePath {
			relPath = strings.TrimPrefix(remotePath, s.basePath)
		}
		relPath = filepath.ToSlash(filepath.Clean(relPath))

		*results = append(*results, ListResult{
			Path:  relPath,
			Size:  f.Size(),
		})
	}
	return nil
}
