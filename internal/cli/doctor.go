package cli

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/gookit/gcli/v3"
	"github.com/gookit/goutil/errorx"

	"github.com/inhere/fakeserver/internal/config"
)

type doctorOptions struct {
	cwd        string
	configFlag string
	envName    string
	out        io.Writer
}

type doctorFinding struct {
	Level   string
	Area    string
	Message string
	Fix     string
}

func newDoctorCmd() *gcli.Command {
	opts := doctorOptions{}
	return &gcli.Command{
		Name: "doctor",
		Desc: "Diagnose fakeserver project configuration and local runtime risks",
		Config: func(cmd *gcli.Command) {
			cmd.StrOpt2(&opts.configFlag, "config,c", "Comma-separated config paths (default: search CWD)")
			cmd.StrOpt2(&opts.envName, "env,e", "Environment segment name")
		},
		Func: func(cmd *gcli.Command, _ []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getwd: %w", err)
			}
			opts.cwd = wd
			opts.out = os.Stdout
			return runDoctor(opts)
		},
	}
}

func runDoctor(opts doctorOptions) error {
	if opts.out == nil {
		opts.out = os.Stdout
	}
	paths := splitConfigPaths(opts.configFlag)
	if len(paths) == 0 {
		paths = existingDefaultConfigPaths(opts.cwd)
	} else {
		paths = resolveDoctorConfigPaths(opts.cwd, paths)
	}
	var findings []doctorFinding
	if len(paths) == 0 {
		findings = append(findings, doctorFinding{
			Level:   "FAIL",
			Area:    "config",
			Message: "no config file found",
			Fix:     "run fakeserver init --full or pass -c <path>",
		})
		return printDoctorFindings(opts.out, findings)
	}

	cfg, err := config.Load(paths, opts.envName, nil)
	if err != nil {
		findings = append(findings, doctorFinding{
			Level:   "FAIL",
			Area:    "config",
			Message: err.Error(),
			Fix:     "fix JSON5 syntax, include paths, or env file errors",
		})
		return printDoctorFindings(opts.out, findings)
	}
	findings = append(findings, doctorFinding{Level: "OK", Area: "config", Message: strings.Join(paths, ", ")})

	validateErrs := config.Validate(cfg)
	bodyFileFailed := false
	for _, verr := range validateErrs {
		area := "validate"
		fix := "fix the reported config validation problem"
		if strings.Contains(verr.Error(), "bodyFile") {
			area = "bodyFile"
			fix = "create the fixture file or update bodyFile to the correct relative path"
			bodyFileFailed = true
		}
		findings = append(findings, doctorFinding{Level: "FAIL", Area: area, Message: verr.Error(), Fix: fix})
	}
	if !bodyFileFailed {
		findings = append(findings, doctorFinding{Level: "OK", Area: "bodyFile", Message: "all configured bodyFile paths exist"})
	}

	if cfg.EnvSource != "" {
		findings = append(findings, doctorFinding{Level: "OK", Area: "env", Message: cfg.EnvSource})
	} else {
		findings = append(findings, doctorFinding{Level: "WARN", Area: "env", Message: "no fakeserver.env.json5 loaded", Fix: "create fakeserver.env.json5 if templates use .env"})
	}

	sources := len(cfg.SourcePaths)
	if cfg.EnvSource != "" && sources > 0 {
		sources--
	}
	if sources < 0 {
		sources = 0
	}
	findings = append(findings, doctorFinding{Level: "OK", Area: "includes", Message: fmt.Sprintf("%d loaded source file(s)", sources)})

	if cfg.Server.AdminEnabled != nil && *cfg.Server.AdminEnabled {
		findings = append(findings, doctorFinding{Level: "OK", Area: "webui", Message: "/__fakeserver/ui/ enabled"})
		if isNonLoopbackListenHost(cfg.Server.Host) {
			if cfg.Server.AdminAllowRemote {
				findings = append(findings, doctorFinding{Level: "WARN", Area: "admin", Message: "non-loopback host with adminAllowRemote=true exposes admin endpoints", Fix: "set server.host to 127.0.0.1 or adminAllowRemote=false"})
			}
		}
	} else {
		findings = append(findings, doctorFinding{Level: "WARN", Area: "webui", Message: "admin/webui disabled", Fix: "set server.adminEnabled=true for local UI access"})
	}

	if err := checkPortAvailable(cfg.Server.Host, cfg.Server.Port); err != nil {
		findings = append(findings, doctorFinding{Level: "FAIL", Area: "port", Message: err.Error(), Fix: "stop the process using this port or choose another --port"})
	} else {
		findings = append(findings, doctorFinding{Level: "OK", Area: "port", Message: fmt.Sprintf("%s:%d available", cfg.Server.Host, cfg.Server.Port)})
	}

	for _, warn := range config.Warn(cfg) {
		findings = append(findings, doctorFinding{Level: "WARN", Area: "proxy", Message: warn})
	}

	return printDoctorFindings(opts.out, findings)
}

func resolveDoctorConfigPaths(cwd string, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if filepath.IsAbs(p) {
			out = append(out, p)
			continue
		}
		out = append(out, filepath.Join(cwd, p))
	}
	return out
}

func existingDefaultConfigPaths(cwd string) []string {
	for _, rel := range config.DefaultPaths() {
		p := filepath.Join(cwd, rel)
		if _, err := os.Stat(p); err == nil {
			return []string{p}
		}
	}
	return nil
}

func checkPortAvailable(host string, port int) error {
	if host == "" {
		host = "127.0.0.1"
	}
	if host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return err
	}
	return ln.Close()
}

func printDoctorFindings(w io.Writer, findings []doctorFinding) error {
	failed := false
	for _, f := range findings {
		if f.Level == "FAIL" {
			failed = true
		}
		fmt.Fprintf(w, "%-4s %-8s %s\n", f.Level, f.Area, f.Message)
		if f.Fix != "" {
			fmt.Fprintf(w, "     fix: %s\n", f.Fix)
		}
	}
	if failed {
		return errorx.Failf(1, "doctor: failed")
	}
	return nil
}
