// Package registry 维护 fakeserver 跨进程的项目注册表与 PID 文件。
// 详见 design §10。
package registry

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Project 是 projects.json 中单条记录，对应 design §10.2 表结构。
type Project struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	ConfigPath string    `json:"configPath"`
	CWD        string    `json:"cwd"`
	Envs       []string  `json:"envs,omitempty"`
	LastEnv    string    `json:"lastEnv,omitempty"`
	LastPort   int       `json:"lastPort,omitempty"`
	LastRunAt  time.Time `json:"lastRunAt"`
	PIDFile    string    `json:"pidFile,omitempty"`
}

// Registry 是 projects.json 顶层结构。
type Registry struct {
	Version      int       `json:"version"`
	LastActiveId string    `json:"lastActiveId,omitempty"`
	Projects     []Project `json:"projects"`
}

// Load 读取 path 指向的 projects.json。
//
// 行为约定（design §10.3）：
//   - 文件不存在 → 返回空 Registry{Version:1} + nil err
//   - 解析失败 → 重命名为 path+".bak-<unix-ts>"，新建空文件，warn 日志，返回空 Registry + nil err
//   - IO 错误（非 NotExist） → 返回 nil + err
func Load(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return emptyRegistry(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		bak := fmt.Sprintf("%s.bak-%d", path, time.Now().Unix())
		if renameErr := os.Rename(path, bak); renameErr != nil {
			fmt.Fprintf(os.Stderr, "warn: registry corrupt at %s and rename to bak failed: %v\n", path, renameErr)
		} else {
			fmt.Fprintf(os.Stderr, "warn: registry corrupt at %s; renamed to %s; using empty registry\n", path, bak)
		}
		return emptyRegistry(), nil
	}
	if reg.Version == 0 {
		reg.Version = 1
	}
	if reg.Projects == nil {
		reg.Projects = []Project{}
	}
	return &reg, nil
}

func emptyRegistry() *Registry {
	return &Registry{Version: 1, Projects: []Project{}}
}

// Save 原子写：tmp → fsync → rename（design §10.3）。
// 自动创建父目录（0700）。
func Save(path string, reg *Registry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if reg.Projects == nil {
		reg.Projects = []Project{}
	}
	if reg.Version == 0 {
		reg.Version = 1
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("open tmp %s: %w", tmp, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("fsync tmp: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename tmp → %s: %w", path, err)
	}
	return nil
}

// ProjectID 由 sha1(configAbsPath) 前 12 hex 位组成（design §10.2）。
// caller 应传绝对路径；非绝对路径会被先 filepath.Abs 化（出错则按原值哈希）。
func ProjectID(configPath string) string {
	abs, err := filepath.Abs(configPath)
	if err != nil {
		abs = configPath
	}
	sum := sha1.Sum([]byte(abs))
	return hex.EncodeToString(sum[:])[:12]
}

// Upsert 把 p 写入 reg：按 id 替换或追加，同时把 lastActiveId 更新为 p.ID。
// 不持久化；caller 在外层 Save。
func Upsert(reg *Registry, p Project) {
	for i := range reg.Projects {
		if reg.Projects[i].ID == p.ID {
			reg.Projects[i] = p
			reg.LastActiveId = p.ID
			return
		}
	}
	reg.Projects = append(reg.Projects, p)
	reg.LastActiveId = p.ID
}

// Get 返回 id 命中的 Project 副本与是否找到。
func Get(reg *Registry, id string) (Project, bool) {
	for _, p := range reg.Projects {
		if p.ID == id {
			return p, true
		}
	}
	return Project{}, false
}

// Remove 按 id 移除一条记录。不存在不报错；若 lastActiveId 命中删除项，自动清空。
func Remove(reg *Registry, id string) {
	out := reg.Projects[:0]
	for _, p := range reg.Projects {
		if p.ID != id {
			out = append(out, p)
		}
	}
	reg.Projects = out
	if reg.LastActiveId == id {
		reg.LastActiveId = ""
	}
}
