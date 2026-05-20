package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gookit/gcli/v3"
	"github.com/gookit/goutil/errorx"

	"github.com/inhere/fakeserver/internal/registry"
)

type useOptions struct {
	regPath string // 测试可注入；空 → userRegistryPath()
	out     io.Writer
	id      string
}

func newUseCmd() *gcli.Command {
	opts := useOptions{out: os.Stdout}
	return &gcli.Command{
		Name: "use",
		Desc: "Mark a registered project as the lastActiveId (does not start the server)",
		Config: func(cmd *gcli.Command) {
			cmd.AddArg("id", "Project ID (or unique prefix) to activate", true, false)
		},
		Func: func(cmd *gcli.Command, _ []string) error {
			opts.id = cmd.Arg("id").String()
			return runUse(opts)
		},
	}
}

func runUse(opts useOptions) error {
	if strings.TrimSpace(opts.id) == "" {
		return errorx.Failf(1, "use: missing project id argument")
	}
	regPath := opts.regPath
	if regPath == "" {
		regPath = userRegistryPath()
	}
	reg, err := registry.Load(regPath)
	if err != nil {
		return errorx.Failf(1, "use: load registry: %s", err.Error())
	}
	if len(reg.Projects) == 0 {
		return errorx.Failf(1, "use: no projects registered; run 'fakeserver serve' first")
	}
	resolved, err := resolveProjectID(reg, opts.id)
	if err != nil {
		return errorx.Failf(1, "use: %s", err.Error())
	}
	reg.LastActiveId = resolved
	if err := registry.WithLock(regPath+".lock", func() error {
		// 重新 Load → Save，避免覆盖并发写入；但因 use 是低频命令，简化为直接 Save。
		return registry.Save(regPath, reg)
	}); err != nil {
		return errorx.Failf(1, "use: save: %s", err.Error())
	}
	fmt.Fprintf(opts.out, "active: %s\n", resolved)
	return nil
}

// resolveProjectID 把 idOrPrefix 解析成完整 project id：
//   - 精确匹配优先
//   - 否则做前缀扫描；唯一前缀 → 命中；多个匹配 → ambiguous
//   - 无匹配 → "no project matches"
func resolveProjectID(reg *registry.Registry, idOrPrefix string) (string, error) {
	for _, p := range reg.Projects {
		if p.ID == idOrPrefix {
			return p.ID, nil
		}
	}
	var matches []string
	for _, p := range reg.Projects {
		if strings.HasPrefix(p.ID, idOrPrefix) {
			matches = append(matches, p.ID)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no project matches %q", idOrPrefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous prefix %q matches %d projects: %s", idOrPrefix, len(matches), strings.Join(matches, ", "))
	}
}
