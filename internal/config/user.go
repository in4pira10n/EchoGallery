package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func hashPassword(password string) (string, error) {
	if len(password) < 6 {
		return "", fmt.Errorf("密码长度不能少于6位")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("生成密码哈希失败: %w", err)
	}
	return string(hash), nil
}

func newUser(username, password string) (User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return User{}, fmt.Errorf("用户名不能为空")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	return User{Username: username, PasswordHash: hash}, nil
}

// AddUser 向配置文件中添加一个新用户
func AddUser(username, password string) error {
	if strings.TrimSpace(username) == "" {
		return fmt.Errorf("用户名不能为空")
	}

	cfg, err := Load()
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			return fmt.Errorf("配置文件不存在，请先运行程序完成初始化")
		}
		return err
	}

	// 检查用户名是否已存在
	for _, u := range cfg.Users {
		if u.Username == username {
			return fmt.Errorf("用户名 '%s' 已存在", username)
		}
	}

	user, err := newUser(username, password)
	if err != nil {
		return err
	}

	cfg.Users = append(cfg.Users, user)

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	return nil
}

// RegisterUser 在已有配置中创建新用户，并为其初始化独立 Profile。
func RegisterUser(cfg *Config, username, password string) (*User, error) {
	if cfg == nil {
		return nil, fmt.Errorf("配置不能为空")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("用户名不能为空")
	}
	for _, u := range cfg.Users {
		if strings.EqualFold(strings.TrimSpace(u.Username), username) {
			return nil, fmt.Errorf("用户名 '%s' 已存在", username)
		}
	}
	user, err := newUser(username, password)
	if err != nil {
		return nil, err
	}
	cfg.Users = append(cfg.Users, user)
	if err := cfg.Save(); err != nil {
		cfg.Users = cfg.Users[:len(cfg.Users)-1]
		return nil, fmt.Errorf("保存配置失败: %w", err)
	}
	profile := NewUserProfileTemplate(cfg)
	if err := SaveProfile(cfg, user.Username, profile); err != nil {
		cfg.Users = cfg.Users[:len(cfg.Users)-1]
		_ = cfg.Save()
		return nil, fmt.Errorf("初始化用户 Profile 失败: %w", err)
	}
	return &user, nil
}

func DeleteUser(username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("用户名不能为空")
	}

	cfg, err := Load()
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			return fmt.Errorf("配置文件不存在，请先运行程序完成初始化")
		}
		return err
	}
	if len(cfg.Users) <= 1 {
		return fmt.Errorf("至少需要保留一个用户，无法删除最后一个用户")
	}

	targetIndex := -1
	targetUsername := ""
	for index, user := range cfg.Users {
		if strings.EqualFold(strings.TrimSpace(user.Username), username) {
			targetIndex = index
			targetUsername = user.Username
			break
		}
	}
	if targetIndex < 0 {
		return fmt.Errorf("用户 '%s' 不存在", username)
	}

	cfg.Users = append(cfg.Users[:targetIndex], cfg.Users[targetIndex+1:]...)
	if strings.EqualFold(strings.TrimSpace(cfg.ActiveProfile), targetUsername) {
		cfg.ActiveProfile = ""
		cfg.ensureActiveProfileAssigned()
	}
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	profileDir, err := cfg.ProfileDir(targetUsername)
	if err == nil {
		_ = os.RemoveAll(profileDir)
	}
	return nil
}

// VerifyPassword 验证用户密码，返回对应的 User
func VerifyPassword(cfg *Config, username, password string) (*User, error) {
	for _, u := range cfg.Users {
		if u.Username == username {
			if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
				return nil, fmt.Errorf("密码错误")
			}
			return &u, nil
		}
	}
	return nil, fmt.Errorf("用户不存在")
}

// RunAddUserWizard 运行命令行 adduser 向导
func RunAddUserWizard() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("添加新用户")
	fmt.Println("-----------------------------------")

	username, err := promptString(reader, "请输入用户名", "")
	if err != nil {
		return err
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("用户名不能为空")
	}

	fmt.Print("请输入密码: ")
	password, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("读���密码失败: %w", err)
	}
	password = strings.TrimSpace(password)

	if err := AddUser(username, password); err != nil {
		return err
	}

	fmt.Printf("用户 '%s' 添加成功\n", username)
	return nil
}

func RunDeleteUserWizard(username string) error {
	if err := DeleteUser(username); err != nil {
		return err
	}
	fmt.Printf("用户 '%s' 删除成功\n", strings.TrimSpace(username))
	return nil
}
