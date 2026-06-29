package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 确保 LocalStorage 实现 Driver 接口
var _ Driver = (*LocalStorage)(nil)

type LocalStorage struct {
	baseDir string
}

type StoredObject struct {
	Path     string
	AbsPath  string
	Size     int64
	Filename string
}

type LatestManifest struct {
	Tag       string `json:"tag"`
	UpdatedAt string `json:"updatedAt"`
}

func NewLocalStorage(baseDir string) (*LocalStorage, error) {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = "data/releases"
	}

	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absBaseDir, 0o755); err != nil {
		return nil, err
	}

	return &LocalStorage{baseDir: absBaseDir}, nil
}

func (s *LocalStorage) Put(ctx context.Context, objectPath string, reader io.Reader) (*StoredObject, error) {
	safePath, err := s.safePath(objectPath)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(safePath), 0o755); err != nil {
		return nil, err
	}

	partialPath := safePath + ".partial"
	file, err := os.Create(partialPath)
	if err != nil {
		return nil, err
	}

	written, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(partialPath)
		return nil, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(partialPath)
		return nil, closeErr
	}

	if err := os.Rename(partialPath, safePath); err != nil {
		_ = os.Remove(partialPath)
		return nil, err
	}

	return &StoredObject{
		Path:     filepath.ToSlash(filepath.Clean(objectPath)),
		AbsPath:  safePath,
		Size:     written,
		Filename: filepath.Base(safePath),
	}, nil
}

func (s *LocalStorage) Open(ctx context.Context, objectPath string) (io.ReadCloser, *StoredObject, error) {
	safePath, err := s.safePath(objectPath)
	if err != nil {
		return nil, nil, err
	}

	file, err := os.Open(safePath)
	if err != nil {
		return nil, nil, err
	}

	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}

	return file, &StoredObject{
		Path:     filepath.ToSlash(filepath.Clean(objectPath)),
		AbsPath:  safePath,
		Size:     stat.Size(),
		Filename: filepath.Base(safePath),
	}, nil
}

func (s *LocalStorage) Delete(ctx context.Context, objectPath string) error {
	safePath, err := s.safePath(objectPath)
	if err != nil {
		return err
	}

	if err := os.Remove(safePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	s.removeEmptyParents(filepath.Dir(safePath))
	return nil
}

func (s *LocalStorage) SetLatestTag(ctx context.Context, provider string, owner string, repo string, tag string) error {
	repositoryPath := filepath.ToSlash(filepath.Join(
		safeSegment(provider),
		safeSegment(owner),
		safeSegment(repo),
	))
	repositoryDir, err := s.safePath(repositoryPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(repositoryDir, 0o755); err != nil {
		return err
	}

	latestPath := filepath.Join(repositoryDir, "latest")
	_ = os.Remove(latestPath)
	if err := os.Symlink(safeSegment(tag), latestPath); err != nil {
		manifestPath := filepath.Join(repositoryDir, "latest.json")
		return s.writeLatestManifest(manifestPath, tag)
	}

	manifestPath := filepath.Join(repositoryDir, "latest.json")
	return s.writeLatestManifest(manifestPath, tag)
}

// Capabilities 声明 Local 存储支持符号链接
func (s *LocalStorage) Capabilities() Capabilities {
	return Capabilities{CanSymlink: true}
}

// List 列举指定前缀下的所有文件（跳过符号链接、.partial 和 latest.json）
func (s *LocalStorage) List(ctx context.Context, prefix string) ([]ListResult, error) {
	var results []ListResult
	searchDir := s.baseDir
	if strings.TrimSpace(prefix) != "" {
		safe, err := s.safePath(prefix)
		if err != nil {
			return nil, err
		}
		searchDir = safe
	}

	err := filepath.WalkDir(searchDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // 跳过无权限等错误
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			return nil
		}

		name := d.Name()
		// 跳过临时文件
		if strings.HasSuffix(name, ".partial") {
			return nil
		}

		// 跳过仓库根目录的 latest.json 元数据文件（github/owner/repo/latest.json）
		// 但不跳过 tag 目录下的同名资产文件（github/owner/repo/TAG/latest.json）
		if name == "latest.json" {
			rel, relErr := filepath.Rel(s.baseDir, path)
			if relErr == nil {
				parts := strings.Split(filepath.ToSlash(filepath.Clean(rel)), "/")
				// provider/owner/repo/latest.json = 4 segments → 元数据，跳过
				// provider/owner/repo/TAG/latest.json = 5+ segments → 资产，保留
				if len(parts) == 4 {
					return nil
				}
			}
		}

		// 跳过符号链接（latest 指向目录）
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}

		relPath, err := filepath.Rel(s.baseDir, path)
		if err != nil {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		results = append(results, ListResult{
			Path:  filepath.ToSlash(filepath.Clean(relPath)),
			Size:  info.Size(),
		})
		return nil
	})

	if err != nil && err != context.Canceled {
		return results, err
	}
	return results, nil
}


func (s *LocalStorage) safePath(objectPath string) (string, error) {
	cleanPath := filepath.Clean(strings.TrimSpace(objectPath))
	if cleanPath == "." || cleanPath == "" {
		return "", errors.New("对象路径不能为空")
	}
	if filepath.IsAbs(cleanPath) || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) || cleanPath == ".." {
		return "", fmt.Errorf("对象路径不安全: %s", objectPath)
	}

	targetPath := filepath.Join(s.baseDir, cleanPath)
	relPath, err := filepath.Rel(s.baseDir, targetPath)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || relPath == ".." {
		return "", fmt.Errorf("对象路径越界: %s", objectPath)
	}

	return targetPath, nil
}

func (s *LocalStorage) removeEmptyParents(startDir string) {
	current := startDir
	for {
		if current == s.baseDir {
			return
		}

		relPath, err := filepath.Rel(s.baseDir, current)
		if err != nil || relPath == "." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || relPath == ".." {
			return
		}

		if err := os.Remove(current); err != nil {
			return
		}
		current = filepath.Dir(current)
	}
}

func (s *LocalStorage) writeLatestManifest(path string, tag string) error {
	manifest := LatestManifest{
		Tag:       tag,
		UpdatedAt: timeNowUTC(),
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')

	partialPath := path + ".partial"
	if err := os.WriteFile(partialPath, payload, 0o644); err != nil {
		return err
	}
	if err := os.Rename(partialPath, path); err != nil {
		_ = os.Remove(partialPath)
		return err
	}

	return nil
}

func safeSegment(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "\\", "_")
	if value == "" || value == "." || value == ".." {
		return "_"
	}
	return value
}

func timeNowUTC() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
