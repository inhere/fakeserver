package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/gookit/gcli/v3"

	"github.com/inhere/fakeserver/internal/registry"
)

type listOptions struct {
	regPath string // 测试可注入；空 → userRegistryPath()
	out     io.Writer
}

func newListCmd() *gcli.Command {
	opts := listOptions{out: os.Stdout}
	return &gcli.Command{
		Name: "list",
		Desc: "List registered fakeserver projects (running / idle status)",
		Func: func(cmd *gcli.Command, _ []string) error {
			return runList(opts)
		},
	}
}

// runList 加载 ~/.config/fakeserver/projects.json，对每条记录探活
// （活进程 running + port；死进程 idle 并删除其 PID 文件），按
// lastActiveId 优先 → lastRunAt 倒序输出 6 列表格（design §10.5）。
func runList(opts listOptions) error {
	regPath := opts.regPath
	if regPath == "" {
		regPath = userRegistryPath()
	}
	reg, err := registry.Load(regPath)
	if err != nil {
		return fmt.Errorf("list: load registry: %w", err)
	}
	if len(reg.Projects) == 0 {
		fmt.Fprintln(opts.out, "no projects registered yet; run 'fakeserver serve' to register the current project.")
		return nil
	}

	type row struct {
		proj   registry.Project
		status string
		port   string
	}
	rows := make([]row, 0, len(reg.Projects))
	dirty := false
	for _, p := range reg.Projects {
		r := row{proj: p, status: "idle", port: "-"}
		if p.PIDFile != "" {
			if pid, port, _, perr := registry.ReadPIDFile(p.PIDFile); perr == nil {
				if registry.IsAlive(pid) {
					r.status = "running"
					r.port = fmt.Sprintf("%d", port)
				} else {
					// 死进程 → 清理 PID 文件
					if rerr := registry.RemovePIDFile(p.PIDFile); rerr != nil {
						fmt.Fprintf(os.Stderr, "warn: cleanup stale pid file %s: %v\n", p.PIDFile, rerr)
					}
					dirty = true
				}
			}
		}
		rows = append(rows, r)
	}

	// 排序：lastActiveId 项目优先 → 其余按 lastRunAt 倒序
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].proj.ID == reg.LastActiveId {
			return true
		}
		if rows[j].proj.ID == reg.LastActiveId {
			return false
		}
		return rows[i].proj.LastRunAt.After(rows[j].proj.LastRunAt)
	})

	tw := tabwriter.NewWriter(opts.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tPORT\tENV\tLAST RUN")
	for _, r := range rows {
		env := r.proj.LastEnv
		if env == "" {
			env = "-"
		}
		lastRun := "-"
		if !r.proj.LastRunAt.IsZero() {
			lastRun = r.proj.LastRunAt.Local().Format("2006-01-02 15:04")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.proj.ID, r.proj.Name, r.status, r.port, env, lastRun)
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("list: flush: %w", err)
	}

	// 若 list 时清理了死 PID 文件，保留 projects.json 本身（PID 文件只是 PIDFile 字段指向的物理文件
	// 被删；Project.PIDFile 字段保留，下次 serve 重新写入会复用）。不需要 Save。
	_ = dirty
	return nil
}
