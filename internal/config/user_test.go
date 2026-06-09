package config

import (
	"os"
	"path/filepath"
	"testing"
)

// --- AddUser 测试 ---

func TestAddUser_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	cfg := validConfig()
	if err := cfg.saveToPath(path); err != nil {
		t.Fatal(err)
	}

	// 临时覆盖 configPath
	origConfigPath := overrideConfigPath(path)
	defer origConfigPath()

	if err := AddUser("alice", "password123"); err != nil {
		t.Fatalf("添加用户失败: %v", err)
	}

	// 验证用户已保存
	loaded, err := loadFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Users) != 1 {
		t.Fatalf("期望 1 个用户，得到 %d", len(loaded.Users))
	}
	if loaded.Users[0].Username != "alice" {
		t.Errorf("期望用户名 alice，得到 %s", loaded.Users[0].Username)
	}
	if loaded.Users[0].PasswordHash == "" {
		t.Error("密码哈希不能为空")
	}
}

func TestAddUser_DuplicateUsername(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	cfg := validConfig()
	if err := cfg.saveToPath(path); err != nil {
		t.Fatal(err)
	}

	origConfigPath := overrideConfigPath(path)
	defer origConfigPath()

	if err := AddUser("alice", "password123"); err != nil {
		t.Fatal(err)
	}
	if err := AddUser("alice", "anotherpass"); err == nil {
		t.Error("重复用户名应该返回错误")
	}
}

func TestAddUser_EmptyUsername(t *testing.T) {
	if err := AddUser("", "password123"); err == nil {
		t.Error("空用户名应该返回错误")
	}
}

func TestAddUser_ShortPassword(t *testing.T) {
	if err := AddUser("alice", "123"); err == nil {
		t.Error("短密码应该返回错误")
	}
}

func TestRegisterUser_CreatesUserAndProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	cfg := validConfig()
	cfg.AppDataDir = filepath.Join(dir, appDataDirName)
	cfg.ActiveProfile = "alice"
	cfg.StoragePath = "/tmp/alice-library"
	cfg.Libraries = []Library{{Name: "Alice", Path: "/tmp/alice-library"}}
	if err := cfg.saveToPath(path); err != nil {
		t.Fatal(err)
	}

	origConfigPath := overrideConfigPath(path)
	defer origConfigPath()

	user, err := RegisterUser(cfg, "bob", "password123")
	if err != nil {
		t.Fatalf("注册用户失败: %v", err)
	}
	if user.Username != "bob" {
		t.Fatalf("期望注册 bob，得到 %s", user.Username)
	}
	if len(cfg.Users) != 1 || cfg.Users[0].Username != "bob" {
		t.Fatalf("期望配置中写入新用户，得到 %+v", cfg.Users)
	}
	loaded, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("重新读取配置失败: %v", err)
	}
	if len(loaded.Users) != 1 || loaded.Users[0].Username != "bob" {
		t.Fatalf("期望新用户已落盘，得到 %+v", loaded.Users)
	}
	profile, err := LoadProfile(cfg, "bob")
	if err != nil {
		t.Fatalf("期望创建用户 Profile: %v", err)
	}
	if profile.StoragePath != "" {
		t.Fatalf("新用户 Profile 不应继承旧用户资源库，得到 %s", profile.StoragePath)
	}
	if len(profile.Libraries) != 0 {
		t.Fatalf("新用户 Profile 不应继承旧用户资源库列表，得到 %+v", profile.Libraries)
	}
}

func TestRegisterUser_DuplicateUsernameCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	cfg := validConfig()
	cfg.Users = []User{{Username: "Alice", PasswordHash: "hash"}}
	if err := cfg.saveToPath(path); err != nil {
		t.Fatal(err)
	}

	origConfigPath := overrideConfigPath(path)
	defer origConfigPath()

	if _, err := RegisterUser(cfg, "alice", "password123"); err == nil {
		t.Fatal("重复用户名应返回错误")
	}
}

func TestDeleteUser_RemovesProfileAndSelectsNewActiveProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	cfg := validConfig()
	cfg.AppDataDir = filepath.Join(dir, appDataDirName)
	cfg.ActiveProfile = "alice"
	cfg.Users = []User{
		{Username: "alice", PasswordHash: "hash-1"},
		{Username: "bob", PasswordHash: "hash-2"},
	}
	cfg.StoragePath = "/tmp/alice-library"
	if err := SaveProfile(cfg, "alice", &Profile{StoragePath: "/tmp/alice-library"}); err != nil {
		t.Fatalf("保存 alice Profile 失败: %v", err)
	}
	if err := SaveProfile(cfg, "bob", &Profile{StoragePath: "/tmp/bob-library"}); err != nil {
		t.Fatalf("保存 bob Profile 失败: %v", err)
	}
	if err := cfg.saveToPath(path); err != nil {
		t.Fatal(err)
	}

	origConfigPath := overrideConfigPath(path)
	defer origConfigPath()

	if err := DeleteUser("alice"); err != nil {
		t.Fatalf("删除用户失败: %v", err)
	}

	loaded, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("重新读取配置失败: %v", err)
	}
	if len(loaded.Users) != 1 || loaded.Users[0].Username != "bob" {
		t.Fatalf("期望仅保留 bob，得到 %+v", loaded.Users)
	}
	if loaded.ActiveProfile != "bob" {
		t.Fatalf("期望 active_profile 切换到 bob，得到 %s", loaded.ActiveProfile)
	}
	profilePath, err := cfg.ProfilePath("alice")
	if err != nil {
		t.Fatalf("读取 alice Profile 路径失败: %v", err)
	}
	if _, err := os.Stat(profilePath); !os.IsNotExist(err) {
		t.Fatalf("期望 alice Profile 被删除，得到 %v", err)
	}
}

func TestUpdateUserCredentials_RenamesUserAndMovesProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	cfg := validConfig()
	cfg.AppDataDir = filepath.Join(dir, appDataDirName)
	cfg.ActiveProfile = "alice"
	user, err := newUser("alice", "oldpass123")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Users = []User{user}
	cfg.StoragePath = "/tmp/alice-library"
	if err := SaveProfile(cfg, "alice", &Profile{StoragePath: "/tmp/alice-library"}); err != nil {
		t.Fatalf("保存 alice Profile 失败: %v", err)
	}
	oldProfilePath, err := cfg.ProfilePath("alice")
	if err != nil {
		t.Fatal(err)
	}
	newProfilePath, err := cfg.ProfilePath("carol")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.saveToPath(path); err != nil {
		t.Fatal(err)
	}

	origConfigPath := overrideConfigPath(path)
	defer origConfigPath()

	finalUsername, err := UpdateUserCredentials("alice", "carol", "newpass123")
	if err != nil {
		t.Fatalf("修改用户失败: %v", err)
	}
	if finalUsername != "carol" {
		t.Fatalf("期望返回 carol，得到 %s", finalUsername)
	}
	loaded, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("重新读取配置失败: %v", err)
	}
	if len(loaded.Users) != 1 || loaded.Users[0].Username != "carol" {
		t.Fatalf("期望用户改名为 carol，得到 %+v", loaded.Users)
	}
	if loaded.ActiveProfile != "carol" {
		t.Fatalf("期望 active_profile=carol，得到 %s", loaded.ActiveProfile)
	}
	if _, err := VerifyPassword(loaded, "carol", "newpass123"); err != nil {
		t.Fatalf("新密码应可登录: %v", err)
	}
	if _, err := os.Stat(oldProfilePath); !os.IsNotExist(err) {
		t.Fatalf("期望旧 Profile 已迁移，得到 %v", err)
	}
	if _, err := os.Stat(newProfilePath); err != nil {
		t.Fatalf("期望新 Profile 存在: %v", err)
	}
}

func TestUpdateUserCredentials_PasswordOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	cfg := validConfig()
	user, err := newUser("alice", "oldpass123")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Users = []User{user}
	if err := cfg.saveToPath(path); err != nil {
		t.Fatal(err)
	}

	origConfigPath := overrideConfigPath(path)
	defer origConfigPath()

	if _, err := UpdateUserCredentials("alice", "", "newpass123"); err != nil {
		t.Fatalf("修改密码失败: %v", err)
	}
	loaded, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("重新读取配置失败: %v", err)
	}
	if _, err := VerifyPassword(loaded, "alice", "newpass123"); err != nil {
		t.Fatalf("新密码应可登录: %v", err)
	}
	if _, err := VerifyPassword(loaded, "alice", "oldpass123"); err == nil {
		t.Fatal("旧密码不应继续可用")
	}
}

// --- VerifyPassword 测试 ---

func TestVerifyPassword_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	cfg := validConfig()
	if err := cfg.saveToPath(path); err != nil {
		t.Fatal(err)
	}

	origConfigPath := overrideConfigPath(path)
	defer origConfigPath()

	if err := AddUser("bob", "mypassword"); err != nil {
		t.Fatal(err)
	}

	loaded, _ := loadFromPath(path)
	user, err := VerifyPassword(loaded, "bob", "mypassword")
	if err != nil {
		t.Fatalf("验证密码失败: %v", err)
	}
	if user.Username != "bob" {
		t.Errorf("期望用户名 bob，得到 %s", user.Username)
	}
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	cfg := validConfig()
	if err := cfg.saveToPath(path); err != nil {
		t.Fatal(err)
	}

	origConfigPath := overrideConfigPath(path)
	defer origConfigPath()

	if err := AddUser("bob", "mypassword"); err != nil {
		t.Fatal(err)
	}

	loaded, _ := loadFromPath(path)
	_, err := VerifyPassword(loaded, "bob", "wrongpassword")
	if err == nil {
		t.Error("错误密码应该返回错误")
	}
}

func TestVerifyPassword_UserNotFound(t *testing.T) {
	cfg := validConfig()
	_, err := VerifyPassword(cfg, "nonexistent", "password")
	if err == nil {
		t.Error("不存在的用户应该返回错误")
	}
}
