package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"echogallery/internal/api"
	"echogallery/internal/config"
	"echogallery/internal/service"
	"echogallery/internal/storage/sqlite"
)

const restartDelayEnv = "ECHOGALLERY_RESTART_DELAY_MS"

func main() {
	if delay := strings.TrimSpace(os.Getenv(restartDelayEnv)); delay != "" {
		if ms, err := strconv.Atoi(delay); err == nil && ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
	}

	// 处理子命令
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "adduser":
			if err := config.RunAddUserWizard(); err != nil {
				fmt.Fprintf(os.Stderr, "错误: %v\n", err)
				os.Exit(1)
			}
			return
		default:
			fmt.Fprintf(os.Stderr, "未知命令: %s\n", os.Args[1])
			os.Exit(1)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		if errors.Is(err, config.ErrConfigNotFound) {
			startSetupServer(api.SetupState{
				Mode:    api.SetupModeInit,
				Message: "未找到 config.json，请先完成网页初始化。",
				Port:    8080,
			})
			return
		}
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("配置加载成功，服务将运行在端口 %d\n", cfg.Port)
	fmt.Printf("图片存储路径: %s\n", cfg.StoragePath)

	dbPath, err := cfg.DatabasePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 计算数据库路径失败: %v\n", err)
		os.Exit(1)
	}
	managedDataDir, err := cfg.ManagedDataDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 计算应用数据目录失败: %v\n", err)
		os.Exit(1)
	}
	trashDir, err := cfg.TrashPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 计算回收站目录失败: %v\n", err)
		os.Exit(1)
	}
	thumbDir, err := cfg.ThumbnailStoragePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 计算缩略图目录失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 创建数据库目录失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(managedDataDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 创建应用数据目录失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(trashDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 创建回收站目录失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 创建缩略图目录失败: %v\n", err)
		os.Exit(1)
	}
	if err := migrateLegacyDatabase(cfg, dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 迁移旧数据库失败: %v\n", err)
		os.Exit(1)
	}
	repo, err := sqlite.New(dbPath)
	if err != nil {
		startSetupServer(api.SetupState{
			Mode:               api.SetupModeLibraryRecovery,
			Message:            "当前资源库无法打开数据库，请选择其他资源库后重启。",
			Libraries:          cfg.Libraries,
			CurrentStoragePath: cfg.StoragePath,
			Port:               cfg.Port,
		})
		return
	}
	defer repo.Close()

	if info, err := os.Stat(cfg.StoragePath); err != nil || !info.IsDir() {
		message := fmt.Sprintf("当前资源库不可用: %s", cfg.StoragePath)
		if err != nil {
			if os.IsPermission(err) || strings.Contains(strings.ToLower(err.Error()), "permission denied") {
				message = fmt.Sprintf("当前资源库无法访问，可能是权限不足: %v。请更换或新建资源库。", err)
			} else {
				message = fmt.Sprintf("当前资源库不可用: %v", err)
			}
		}
		startSetupServer(api.SetupState{
			Mode:               api.SetupModeLibraryRecovery,
			Message:            message,
			Libraries:          cfg.Libraries,
			CurrentStoragePath: cfg.StoragePath,
			Port:               cfg.Port,
		})
		return
	}
	if err := repo.CheckStoragePathConsistency(cfg.StoragePath, managedDataDir); err != nil {
		startSetupServer(api.SetupState{
			Mode:               api.SetupModeLibraryRecovery,
			Message:            fmt.Sprintf("当前资源库存在问题: %v", err),
			Libraries:          cfg.Libraries,
			CurrentStoragePath: cfg.StoragePath,
			Port:               cfg.Port,
		})
		return
	}

	photoService := service.NewPhotoService(repo, cfg.StoragePath, managedDataDir, trashDir)
	photoService.SetThumbnailRoot(thumbDir)
	photoService.SetThumbnailSize(cfg.ThumbnailSize)
	if len(cfg.Users) > 0 {
		summary, err := photoService.ImportExistingPhotos(1, func(done, total int) {
			if total == 0 {
				return
			}
			const width = 28
			filled := done * width / total
			if filled < 0 {
				filled = 0
			}
			if filled > width {
				filled = width
			}
			fmt.Printf("\r资源库扫描中 [%s%s] %d/%d", strings.Repeat("#", filled), strings.Repeat("-", width-filled), done, total)
			if done == total {
				fmt.Print("\n")
			}
		})
		if err != nil {
			if os.IsPermission(err) || strings.Contains(strings.ToLower(err.Error()), "permission denied") {
				startSetupServer(api.SetupState{
					Mode:               api.SetupModeLibraryRecovery,
					Message:            fmt.Sprintf("当前资源库无法访问，可能是权限不足: %v。请更换或新建资源库。", err),
					Libraries:          cfg.Libraries,
					CurrentStoragePath: cfg.StoragePath,
					Port:               cfg.Port,
				})
				return
			}
			fmt.Fprintf(os.Stderr, "错误: 导入历史图片失败: %v\n", err)
			os.Exit(1)
		}
		if summary.Imported > 0 {
			fmt.Printf("已导入 %d 张历史图片\n", summary.Imported)
		}
	}
	app := api.NewRouterWithStaticWithRestart(cfg, webFS, photoService, restartCurrentProcess)

	addr := fmt.Sprintf(":%d", cfg.Port)
	if host := preferredLANIP(); host != "" {
		fmt.Printf("HTTP 服务已启动: http://%s%s\n", host, addr)
	} else {
		fmt.Printf("HTTP 服务已启动: http://127.0.0.1%s\n", addr)
	}
	if err := http.ListenAndServe(addr, app); err != nil {
		fmt.Fprintf(os.Stderr, "错误: HTTP 服务启动失败: %v\n", err)
		os.Exit(1)
	}
}

func startSetupServer(state api.SetupState) {
	port := state.Port
	if port <= 0 {
		port = 8080
	}
	addr := fmt.Sprintf(":%d", port)
	app := api.NewSetupRouterWithStatic(webFS, state, restartCurrentProcess)
	fmt.Printf("EchoGallery 设置服务已启动: http://127.0.0.1%s\n", addr)
	if host := preferredLANIP(); host != "" {
		fmt.Printf("局域网访问地址: http://%s%s\n", host, addr)
	}
	if err := http.ListenAndServe(addr, app); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 设置服务启动失败: %v\n", err)
		os.Exit(1)
	}
}

func restartCurrentProcess() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Dir, _ = os.Getwd()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), restartDelayEnv+"=1200")
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		os.Exit(0)
	}()
	return nil
}

func migrateLegacyDatabase(cfg *config.Config, newDBPath string) error {
	if _, err := os.Stat(newDBPath); err == nil {
		return nil
	}

	legacyAppDBPaths, err := cfg.LegacyDatabasePaths()
	if err != nil {
		return err
	}
	candidates := append([]string{}, legacyAppDBPaths...)
	candidates = append(candidates, filepath.Join(cfg.StoragePath, "photoalbum.db"))

	for _, oldDBPath := range candidates {
		if oldDBPath == "" || oldDBPath == newDBPath {
			continue
		}

		matches, err := legacyDatabaseMatchesStoragePath(oldDBPath, cfg.StoragePath)
		if err != nil {
			return err
		}
		if !matches {
			continue
		}

		for _, suffix := range []string{"", "-wal", "-shm"} {
			oldPath := oldDBPath + suffix
			if _, err := os.Stat(oldPath); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return err
			}
			newPath := newDBPath + suffix
			if err := os.Rename(oldPath, newPath); err != nil {
				return err
			}
		}
		return nil
	}
	return nil
}

func legacyDatabaseMatchesStoragePath(dbPath, storagePath string) (bool, error) {
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return false, err
	}
	defer db.Close()

	var exists int
	if err := db.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name='app_meta'`).Scan(&exists); err != nil {
		return false, err
	}
	if exists == 0 {
		return true, nil
	}

	var prev string
	err = db.QueryRow(`SELECT value FROM app_meta WHERE key = 'storage_path'`).Scan(&prev)
	if err != nil {
		if err == sql.ErrNoRows {
			return true, nil
		}
		return false, err
	}

	return config.NormalizeStoragePath(prev) == config.NormalizeStoragePath(storagePath), nil
}

func preferredLANIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}

	var fallback string
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP == nil || ipNet.IP.IsLoopback() {
			continue
		}

		ip := ipNet.IP.To4()
		if ip == nil || !isPrivateIPv4(ip) {
			continue
		}

		if ip[0] == 192 && ip[1] == 168 {
			return ip.String()
		}
		if fallback == "" {
			fallback = ip.String()
		}
	}

	return fallback
}

func isPrivateIPv4(ip net.IP) bool {
	return ip[0] == 10 ||
		(ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) ||
		(ip[0] == 192 && ip[1] == 168)
}
