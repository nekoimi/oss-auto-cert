package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/nekoimi/oss-auto-cert/internal/app"
	"github.com/nekoimi/oss-auto-cert/internal/config"
	"github.com/nekoimi/oss-auto-cert/internal/logging"
	"github.com/nekoimi/oss-auto-cert/internal/notification"
)

// Version 可在构建时通过 -ldflags 注入。
var Version = "dev"

// Normalize 只转换已知旧参数，不修改参数值和 -- 后的内容。
func Normalize(args []string) ([]string, error) {
	result := make([]string, 0, len(args))
	seen := map[string]bool{}
	expectsValue := false
	terminated := false
	valueFlags := map[string]bool{"config": true, "log-level": true, "output": true, "color": true}
	for _, arg := range args {
		if terminated {
			result = append(result, arg)
			continue
		}
		if expectsValue {
			result = append(result, arg)
			expectsValue = false
			continue
		}
		if arg == "--" {
			terminated = true
			result = append(result, arg)
			continue
		}
		name, value, hasEqual := strings.Cut(arg, "=")
		switch name {
		case "-config", "-log-level", "-output":
			name = "-" + name
		case "-help":
			name = "--help"
		case "-h":
			name = "--help"
		case "-v":
			name = "--version"
		}
		normalized := name
		if hasEqual {
			normalized += "=" + value
		}
		if strings.HasPrefix(name, "--") {
			key := strings.TrimPrefix(name, "--")
			if seen[key] {
				return nil, fmt.Errorf("参数 --%s 重复指定", key)
			}
			seen[key] = true
			expectsValue = valueFlags[key] && !hasEqual
		}
		result = append(result, normalized)
	}
	return result, nil
}

// NewRoot 每次创建独立的命令树，不在构造阶段读取配置或访问网络。
func NewRoot(out, errOut io.Writer) *cobra.Command {
	return newRoot(out, errOut, logging.New(errOut))
}

func newRoot(out, errOut io.Writer, logger *logrus.Logger) *cobra.Command {
	var path, level, color string
	root := &cobra.Command{Use: "oss-auto-cert", Short: "阿里云 OSS / CDN 证书自动管理", Version: Version, SilenceErrors: true, SilenceUsage: true, Args: cobra.NoArgs}
	root.SetOut(out)
	root.SetErr(errOut)
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().StringVar(&path, "config", "", "配置文件路径（TOML 或旧 YAML）")
	root.PersistentFlags().StringVar(&level, "log-level", "info", "日志等级 trace/debug/info/warn/error")
	root.PersistentFlags().StringVar(&color, "color", "auto", "日志颜色 auto/always/never")
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return app.Invalid(logging.Configure(logger, level, color))
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return app.Invalid(err) })
	load := func(ignoreEnv bool) (config.Result, error) {
		r, err := config.Load(config.LoadOptions{Path: path, IgnoreEnv: ignoreEnv})
		if err != nil {
			return r, app.Invalid(err)
		}
		for _, w := range r.Warnings {
			logger.Warn(w)
		}
		return r, nil
	}
	execute := func(cmd *cobra.Command, o app.Options) error {
		r, err := load(false)
		if err != nil {
			return err
		}
		return app.Run(cmd.Context(), r.Config, o, logger)
	}
	root.RunE = func(cmd *cobra.Command, args []string) error { return execute(cmd, app.Options{}) }
	var options app.Options
	run := &cobra.Command{Use: "run", Short: "常驻或执行一轮巡检", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if options.Force && !options.Once && !options.DryRun {
			return app.Invalid(fmt.Errorf("--force 必须与 --once 或 --dry-run 配合"))
		}
		return execute(cmd, options)
	}}
	run.Flags().BoolVar(&options.Once, "once", false, "执行一轮后退出")
	run.Flags().BoolVar(&options.DryRun, "dry-run", false, "只读展示计划，不申请和部署证书")
	run.Flags().BoolVar(&options.Force, "force", false, "强制签发所选域名，保留部署恢复策略")
	root.AddCommand(run)
	root.AddCommand(&cobra.Command{Use: "check", Short: "只读检查资源和证书，写权限及实际 challenge 未验证", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { return execute(cmd, app.Options{DryRun: true}) }})
	root.AddCommand(&cobra.Command{Use: "status", Short: "查看本地运行快照（不是云端实时状态）", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		r, err := load(false)
		if err != nil {
			return err
		}
		return app.Status(r.Config, out)
	}})
	root.AddCommand(&cobra.Command{Use: "version", Short: "显示版本", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { _, err := fmt.Fprintln(out, Version); return err }})
	configCmd := &cobra.Command{Use: "config", Short: "配置生成、校验及迁移", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() }}
	var initOutput string
	initCmd := &cobra.Command{Use: "init", Short: "生成最小 TOML 配置，不覆盖已有文件", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := exclusiveWrite(initOutput, []byte(minimalConfig)); err != nil {
			return app.Invalid(err)
		}
		_, err := fmt.Fprintf(out, "已生成 %s，请修改邮箱和 Bucket 后执行 config validate --config=%s\n", initOutput, initOutput)
		return err
	}}
	initCmd.Flags().StringVar(&initOutput, "output", "config.toml", "输出文件路径")
	configCmd.AddCommand(initCmd)
	configCmd.AddCommand(&cobra.Command{Use: "validate", Short: "离线校验配置与通知模板", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		r, err := load(false)
		if err != nil {
			return err
		}
		for _, err := range notification.ValidateTemplates(r.Config.Notifications) {
			return app.Invalid(err)
		}
		if r.Config.ACME.Email == "" {
			logger.Warn("执行云端命令前需设置 acme.email 或 ACME_EMAIL")
		}
		_, err = fmt.Fprintf(out, "配置有效：%s，%d 个 Bucket；未检查凭证、写权限及域名可达性\n", r.Path, len(r.Config.Buckets))
		return err
	}})
	var output string
	migrate := &cobra.Command{Use: "migrate", Short: "将旧 YAML 导出为 TOML，默认输出至标准输出", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		r, err := load(true)
		if err != nil {
			return err
		}
		if ext := strings.ToLower(filepath.Ext(r.Path)); ext != ".yaml" && ext != ".yml" {
			return app.Invalid(fmt.Errorf("migrate 只接受旧 YAML 文件"))
		}
		raw, err := config.Encode(r.Config)
		if err != nil {
			return err
		}
		for _, err := range notification.ValidateTemplates(r.Config.Notifications) {
			logger.WithError(err).Warn("通知模板需要调整")
		}
		logger.Warn("迁移保留 first 域名范围及旧续期策略，不固化环境覆盖；到期比较改为真实时刻")
		if output == "" {
			_, err = out.Write(raw)
			return err
		}
		if err := exclusiveWrite(output, raw); err != nil {
			return app.Invalid(err)
		}
		logger.WithField("output", output).Info("配置已导出；新旧文件并存时请使用 --config 指定新配置路径")
		return nil
	}}
	migrate.Flags().StringVar(&output, "output", "", "输出文件；默认标准输出")
	configCmd.AddCommand(migrate)
	root.AddCommand(configCmd)
	root.SetVersionTemplate("{{.Version}}\n")
	root.SetHelpCommand(&cobra.Command{Use: "help [命令]", Short: "显示命令帮助", RunE: func(cmd *cobra.Command, args []string) error {
		target, rest, err := root.Find(args)
		if err != nil || len(rest) > 0 {
			return app.Invalid(fmt.Errorf("未知帮助命令"))
		}
		return target.Help()
	}})
	root.SetHelpTemplate(`{{with (or .Long .Short)}}{{.}}

{{end}}用法：
  {{.UseLine}}
{{if .HasAvailableSubCommands}}
命令：
{{range .Commands}}{{if .IsAvailableCommand}}  {{rpad .Name .NamePadding }} {{.Short}}
{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}
选项：
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}{{if .HasAvailableInheritedFlags}}
全局选项：
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}
使用 oss-auto-cert help [命令] 查看详细说明。
`)
	localizeFlags(root)
	return root
}

func localizeFlags(cmd *cobra.Command) {
	cmd.InitDefaultHelpFlag()
	if flag := cmd.Flags().Lookup("help"); flag != nil {
		flag.Usage = "显示帮助"
	}
	cmd.InitDefaultVersionFlag()
	if flag := cmd.Flags().Lookup("version"); flag != nil {
		flag.Usage = "显示版本"
	}
	for _, child := range cmd.Commands() {
		localizeFlags(child)
	}
}

func exclusiveWrite(path string, raw []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("无法创建输出文件（不会覆盖已有文件）: %w", err)
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Execute 执行命令并统一退出码和输出行为。
func Execute(ctx context.Context, args []string, out, errOut io.Writer) int {
	logger := logging.New(errOut)
	normalized, err := Normalize(args)
	if err == nil {
		cmd := newRoot(out, errOut, logger)
		cmd.SetArgs(normalized)
		err = cmd.ExecuteContext(ctx)
	} else {
		err = app.Invalid(err)
	}
	if err == nil {
		return 0
	}
	logger.WithError(err).Error("命令执行失败")
	var coded *app.Error
	if errors.As(err, &coded) {
		return coded.Code
	}
	return 2
}

// Main 处理进程信号并执行命令。
func Main() int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return Execute(ctx, os.Args[1:], os.Stdout, os.Stderr)
}

const minimalConfig = `# oss-auto-cert TOML 配置；凭证使用 OSS_ACCESS_KEY_ID / OSS_ACCESS_KEY_SECRET
[acme]
email = "admin@example.com"

[[buckets]]
name = "my-bucket"
endpoint = "oss-cn-hangzhou.aliyuncs.com"
domains = ["static.example.com"]
`
