package config

import (
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	DataDir            string
	TaskDB             string
	ConfigDB           string
	DataDB             string
	AdminToken         string
	SetupToken         string
	Origins            []string
	CookieSecure       bool
	TrustedProxyCIDRs  []netip.Prefix
	TrustedProxySocket string
	ListenAddress      string
}

func Load() (Config, error) {
	root, err := projectRoot()
	if err != nil {
		return Config{}, err
	}
	dataDir, err := pathValue("SUB2API_CONSOLE_DATA_DIR", filepath.Join(root, "data"))
	if err != nil {
		return Config{}, err
	}
	taskDB, err := pathValue("SUB2API_CONSOLE_TASK_DB", filepath.Join(dataDir, "tasks.sqlite3"))
	if err != nil {
		return Config{}, err
	}
	configDB, err := pathValue("SUB2API_CONSOLE_CONFIG_DB", filepath.Join(dataDir, "console-config.sqlite3"))
	if err != nil {
		return Config{}, err
	}
	dataDB, err := pathValue("SUB2API_CONSOLE_DATA_DB", filepath.Join(dataDir, "sub2api-console.sqlite3"))
	if err != nil {
		return Config{}, err
	}
	origins := splitNonEmpty(envOrDefault(
		"SUB2API_CONSOLE_FRONTEND_ORIGINS",
		"http://localhost:3004,http://127.0.0.1:3004",
	))
	listenAddress := strings.TrimSpace(envOrDefault("SUB2API_CONSOLE_LISTEN", "0.0.0.0:8080"))
	if listenAddress == "" {
		return Config{}, errors.New("SUB2API_CONSOLE_LISTEN 不能为空")
	}
	cookieSecure, err := boolEnv("SUB2API_CONSOLE_COOKIE_SECURE", false)
	if err != nil {
		return Config{}, err
	}
	trustedProxyCIDRs, err := prefixListEnv("SUB2API_CONSOLE_TRUSTED_PROXY_CIDRS")
	if err != nil {
		return Config{}, err
	}
	trustedProxySocket := strings.TrimSpace(os.Getenv("SUB2API_CONSOLE_TRUSTED_PROXY_SOCKET"))
	if trustedProxySocket != "" {
		if !filepath.IsAbs(trustedProxySocket) || filepath.Clean(trustedProxySocket) == string(filepath.Separator) {
			return Config{}, errors.New("SUB2API_CONSOLE_TRUSTED_PROXY_SOCKET 必须是绝对文件路径")
		}
		trustedProxySocket = filepath.Clean(trustedProxySocket)
	}
	setupToken := strings.TrimSpace(os.Getenv("SUB2API_CONSOLE_SETUP_TOKEN"))
	if setupToken != "" && len(setupToken) < 32 {
		return Config{}, errors.New("SUB2API_CONSOLE_SETUP_TOKEN 至少需要 32 个字符")
	}
	return Config{
		DataDir:            dataDir,
		TaskDB:             taskDB,
		ConfigDB:           configDB,
		DataDB:             dataDB,
		AdminToken:         strings.TrimSpace(os.Getenv("SUB2API_CONSOLE_CONSOLE_ADMIN_TOKEN")),
		SetupToken:         setupToken,
		Origins:            origins,
		CookieSecure:       cookieSecure,
		TrustedProxyCIDRs:  trustedProxyCIDRs,
		TrustedProxySocket: trustedProxySocket,
		ListenAddress:      listenAddress,
	}, nil
}

func prefixListEnv(name string) ([]netip.Prefix, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil, nil
	}
	values := splitNonEmpty(raw)
	result := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, errors.New(name + " 必须是以逗号分隔的有效 CIDR")
		}
		result = append(result, prefix.Masked())
	}
	return result, nil
}

func projectRoot() (string, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	current, err := filepath.Abs(workingDirectory)
	if err != nil {
		return "", err
	}
	for {
		if fileExists(filepath.Join(current, "frontend", "package.json")) && fileExists(filepath.Join(current, "backend")) {
			return current, nil
		}
		if filepath.Base(current) == "backend" && fileExists(filepath.Join(current, "go.mod")) {
			return filepath.Dir(current), nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return workingDirectory, nil
		}
		current = parent
	}
}

func pathValue(name string, fallback string) (string, error) {
	raw, present := os.LookupEnv(name)
	if present && strings.TrimSpace(raw) == "" {
		return "", errors.New(name + " 不能是空字符串；删除配置项以使用默认 data 目录")
	}
	value := fallback
	if present {
		value = strings.TrimSpace(raw)
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func envOrDefault(name string, fallback string) string {
	if value, present := os.LookupEnv(name); present {
		return value
	}
	return fallback
}

func boolEnv(name string, fallback bool) (bool, error) {
	raw, present := os.LookupEnv(name)
	if !present {
		return fallback, nil
	}
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, errors.New(name + " 必须是 true 或 false")
	}
	return value, nil
}

func splitNonEmpty(raw string) []string {
	result := make([]string, 0)
	for _, item := range strings.Split(raw, ",") {
		if normalized := strings.TrimSpace(item); normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
