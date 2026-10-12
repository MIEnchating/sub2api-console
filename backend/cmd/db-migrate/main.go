package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtransfer"
)

func main() {
	var options dbtransfer.Options
	var driver, reportPath string
	flag.StringVar(&options.Sources.Config, "config", "", "源配置 SQLite 绝对路径")
	flag.StringVar(&options.Sources.Business, "business", "", "源业务 SQLite 绝对路径")
	flag.StringVar(&options.Sources.Tasks, "tasks", "", "源任务 SQLite 绝对路径")
	flag.StringVar(&options.BackupDirectory, "backup-dir", "", "新的私有备份目录绝对路径")
	flag.StringVar(&driver, "driver", "postgres", "目标 postgres 或 mysql")
	flag.StringVar(&reportPath, "report", "", "新建校验报告路径（0600，不覆盖现有文件）")
	flag.BoolVar(&options.Stopped, "source-stopped", false, "确认源控制台已停止，迁移期间不启动")
	flag.Parse()
	options.Target = dbconn.Config{Driver: dbconn.Driver(driver), DSN: os.Getenv("SUB2API_CONSOLE_DB_DSN")}
	if reportPath == "" {
		fmt.Fprintln(os.Stderr, "必须指定 -report 保存校验报告")
		os.Exit(2)
	}
	reportFile, err := os.OpenFile(reportPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法创建新校验报告")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	report, transferErr := dbtransfer.Transfer(ctx, options)
	cancel()
	encoder := json.NewEncoder(reportFile)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(report)
	syncErr := reportFile.Sync()
	closeErr := reportFile.Close()
	if transferErr != nil {
		fmt.Fprintln(os.Stderr, transferErr)
		os.Exit(1)
	}
	if encodeErr != nil || syncErr != nil || closeErr != nil {
		fmt.Fprintln(os.Stderr, "导入已提交，但校验报告写入失败；请先检查目标状态")
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "导入和内容校验完成，报告已保存；原始 SQLite 备份保留。")
}
