package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/joho/godotenv"
)

var (
	channelID       = flag.Int("channel-id", 0, "要修复的渠道 ID")
	nodeName        = flag.String("node-name", "", "创建缺失 quota_data 行时使用的节点名称；留空使用当前节点")
	startValue      = flag.String("start", "", "开始时间（Unix 秒、RFC3339 或 YYYY-MM-DD[ HH:MM:SS]）")
	endValue        = flag.String("end", "", "结束时间（Unix 秒、RFC3339 或 YYYY-MM-DD[ HH:MM:SS]）")
	timezoneName    = flag.String("timezone", "Asia/Shanghai", "不带时区时间使用的时区")
	execute         = flag.Bool("execute", false, "执行数据库写入；默认只预览")
	createMissing   = flag.Bool("create-missing", false, "为没有 quota_data 行的日志聚合创建新行")
	replaceExisting = flag.Bool("replace", false, "用日志聚合值覆盖已有 token 明细（需确认日志完整）")
)

func main() {
	if hasHelpFlag(os.Args[1:]) {
		printRepairUsage()
		return
	}
	flag.Parse()
	if err := runRepair(); err != nil {
		fail(err)
	}
}

func runRepair() error {
	if err := loadDotEnv(); err != nil {
		return fmt.Errorf("加载 .env 失败: %w", err)
	}
	// The repair command must never run migrations or system-task runners.
	_ = os.Setenv("NODE_TYPE", "slave")
	common.InitEnv()

	start, end, err := parseRepairRange()
	if err != nil {
		return err
	}
	if err := model.InitDB(); err != nil {
		return fmt.Errorf("初始化主数据库失败: %w", err)
	}
	if err := model.InitLogDB(); err != nil {
		return fmt.Errorf("初始化日志数据库失败: %w", err)
	}
	defer func() { _ = model.CloseDB() }()
	mode := "dry-run"
	if *execute {
		mode = "execute"
	}
	fmt.Fprintf(os.Stderr, "开始修复 quota_data：channel_id=%d start=%d end=%d mode=%s\n", *channelID, start, end, mode)
	if !*replaceExisting {
		fmt.Fprintln(os.Stderr, "说明：默认仅填充现有行中为空的 token 明细；如需按日志完整重算，请追加 --replace。")
	}
	if *createMissing {
		fmt.Fprintln(os.Stderr, "警告：--create-missing 无法从 logs 判断节点归属，多节点部署请先确认并做好去重。")
	}
	if *execute {
		fmt.Fprintln(os.Stderr, "警告：执行写入前请暂停其他会写入 quota_data 的服务，并确认已备份数据库。")
	}
	createdNodeName := strings.TrimSpace(*nodeName)
	if createdNodeName == "" {
		createdNodeName = common.NodeName
	}

	report, err := model.RepairQuotaData(context.Background(), model.QuotaDataRepairRequest{
		ChannelID:       *channelID,
		NodeName:        createdNodeName,
		StartTimestamp:  start,
		EndTimestamp:    end,
		Apply:           *execute,
		CreateMissing:   *createMissing,
		ReplaceExisting: *replaceExisting,
	})
	if err != nil {
		return err
	}
	encoded, err := common.Marshal(report)
	if err != nil {
		return fmt.Errorf("编码修复报告失败: %w", err)
	}
	fmt.Printf("mode=%s\n%s\n", mode, encoded)
	return nil
}

func parseRepairRange() (int64, int64, error) {
	location, err := time.LoadLocation(strings.TrimSpace(*timezoneName))
	if err != nil {
		return 0, 0, fmt.Errorf("加载时区失败: %w", err)
	}
	start, err := parseRepairTime(*startValue, location, false)
	if err != nil {
		return 0, 0, fmt.Errorf("解析开始时间失败: %w", err)
	}
	end, err := parseRepairTime(*endValue, location, true)
	if err != nil {
		return 0, 0, fmt.Errorf("解析结束时间失败: %w", err)
	}
	return start, end, nil
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func printRepairUsage() {
	flag.CommandLine.SetOutput(os.Stdout)
	fmt.Println("用法: go run ./cmd/quota-data-repair [选项]")
	fmt.Println("默认只预览；确认报告后再追加 --execute 写入数据库。")
	flag.PrintDefaults()
}

func loadDotEnv() error {
	if err := godotenv.Load(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func parseRepairTime(raw string, location *time.Location, endOfDay bool) (int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, errors.New("时间不能为空")
	}
	if timestamp, err := strconv.ParseInt(value, 10, 64); err == nil {
		if timestamp > 0 {
			return timestamp, nil
		}
		return 0, errors.New("Unix 时间必须为正数")
	}
	if timestamp, err := time.Parse(time.RFC3339, value); err == nil {
		return timestamp.Unix(), nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
		timestamp, err := time.ParseInLocation(layout, value, location)
		if err != nil {
			continue
		}
		if endOfDay && layout == "2006-01-02" {
			timestamp = timestamp.Add(24*time.Hour - time.Second)
		}
		return timestamp.Unix(), nil
	}
	return 0, fmt.Errorf("不支持的时间格式 %q", value)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "quota-data-repair:", err)
	os.Exit(1)
}
