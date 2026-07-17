package storage

import (
	"context"
	"errors"
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

	// 确保目录存在（容错处理 OpenList/AList 的 405/423）
	if err := s.mkdirAllSafe(dir); err != nil {
		return nil, err
	}

	// 尝试流式写入：WriteStream 内部会调用 createParentCollection，
	// 对 OpenList/AList 等服务可能因目录已存在返回 405/423。
	// 此时 reader 尚未被消费，可安全回退到缓冲写入。
	cr := &countingReader{reader: reader}
	err := s.client.WriteStream(remotePath, cr, 0o644)
	if err != nil {
		if !isMkdirAcceptable(err) {
			return nil, fmt.Errorf("上传到 WebDAV 失败: %w", err)
		}
		// 流式因 createParentCollection 的 405/423 失败（reader 未被消费），回退缓冲模式
		data, readErr := io.ReadAll(reader)
		if readErr != nil {
			return nil, fmt.Errorf("读取上传数据失败: %w", readErr)
		}
		if writeErr := s.client.Write(remotePath, data, 0o644); writeErr != nil {
			return nil, fmt.Errorf("上传到 WebDAV 失败: %w", writeErr)
		}
		return &StoredObject{
			Path:     filepath.ToSlash(filepath.Clean(objectPath)),
			AbsPath:  remotePath,
			Size:     int64(len(data)),
			Filename: filepath.Base(remotePath),
		}, nil
	}

	return &StoredObject{
		Path:     filepath.ToSlash(filepath.Clean(objectPath)),
		AbsPath:  remotePath,
		Size:     cr.n,
		Filename: filepath.Base(remotePath),
	}, nil
}

func (s *WebDAVStorage) Open(ctx context.Context, objectPath string) (io.ReadCloser, *StoredObject, error) {
	remotePath := s.remotePath(objectPath)

	// 先尝试获取文件大小（一次 PROPFIND），用于设置 Content-Length
	// Stat 失败不阻断流式读取（某些 WebDAV 服务 PROPFIND 支持有限）
	size := int64(-1)
	if info, err := s.client.Stat(remotePath); err == nil {
		size = info.Size()
	}

	// 流式读取替代原来 Read 全量缓冲到内存的方式
	rc, err := s.client.ReadStream(remotePath)
	if err != nil {
		return nil, nil, fmt.Errorf("从 WebDAV 读取失败: %w", err)
	}

	return rc, &StoredObject{
		Path:     filepath.ToSlash(filepath.Clean(objectPath)),
		AbsPath:  remotePath,
		Size:     size,
		Filename: filepath.Base(remotePath),
	}, nil
}

func (s *WebDAVStorage) Delete(ctx context.Context, objectPath string) error {
	remotePath := s.remotePath(objectPath)

	if err := s.client.Remove(remotePath); err != nil {
		return fmt.Errorf("从 WebDAV 删除失败: %w", err)
	}

	// 删除文件后，尝试向上清理空目录（版本目录、仓库目录等）
	s.removeEmptyDirs(objectPath)

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

		// 跳过仓库根目录的 latest.json 元数据文件
		 // provider/owner/repo/latest.json = 元数据，跳过
		 // provider/owner/repo/TAG/latest.json = 资产，保留
		 name := f.Name()
		 if name == "latest.json" {
		  relPath := strings.TrimPrefix(remotePath, s.basePath+"/")
		  if relPath == remotePath {
		   relPath = strings.TrimPrefix(remotePath, s.basePath)
		  }
		  parts := strings.Split(filepath.ToSlash(filepath.Clean(relPath)), "/")
		  if len(parts) == 4 {
		   continue
		  }
		 }

		// 去掉 basePath 前缀，返回相对路径
		relPath := strings.TrimPrefix(remotePath, s.basePath+"/")
		if relPath == remotePath {
			relPath = strings.TrimPrefix(remotePath, s.basePath)
		}
		relPath = filepath.ToSlash(filepath.Clean(relPath))

		*results = append(*results, ListResult{
			Path: relPath,
			Size: f.Size(),
		})
	}
	return nil
}

// removeEmptyDirs 删除文件后向上清理空目录
// 路径格式: github/owner/repo/tag/filename → 依次检查 tag/、repo/、owner/ 目录是否为空
func (s *WebDAVStorage) removeEmptyDirs(objectPath string) {
	// objectPath 示例: github/owner/repo/tag/filename.zip
	parts := strings.Split(filepath.ToSlash(filepath.Clean(objectPath)), "/")
	// 需要检查的目录层级: .../tag/、.../repo/、.../owner/
	// 从倒数第2层（tag目录）开始，到第2层（owner目录）
	for depth := len(parts) - 1; depth >= 2; depth-- {
		dirPath := filepath.ToSlash(filepath.Join(s.basePath, filepath.Join(parts[:depth]...)))
		if s.isRemoteDirEmpty(dirPath) {
			removed := false
			// 不同 WebDAV 服务器对删除目录的路径格式要求不同
			// 先尝试不带斜杠，再尝试带斜杠
			for _, p := range []string{dirPath, dirPath + "/"} {
				if err := s.client.Remove(p); err == nil {
					removed = true
					break
				}
			}
			if !removed {
				// 删除失败可能是目录非空或无权限，停止向上清理
				return
			}
		} else {
			// 目录非空，停止向上清理
			return
		}
	}
}

// isRemoteDirEmpty 检查 WebDAV 远程目录是否为空（只看直接子项，不递归）
func (s *WebDAVStorage) isRemoteDirEmpty(dirPath string) bool {
	files, err := s.client.ReadDir(dirPath)
	if err != nil {
		// 目录不存在或无法读取，视为非空（安全起见不删除）
		return false
	}
	// 过滤掉 latest.json 元数据文件
	for _, f := range files {
		if f.Name() != "latest.json" {
			return false
		}
	}
	return true
}
