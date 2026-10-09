package config

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type InitialConfigParams struct {
	AppName       string
	Port          int
	Libraries     []Library
	StoragePath   string
	ThumbnailDir  string
	ThumbnailSize int
	TrashDir      string
	Username      string
	Password      string
}

func CreateInitialConfig(params InitialConfigParams) (*Config, error) {
	if params.Port <= 0 || params.Port > 65535 {
		params.Port = 8080
	}
	libraries := append([]Library(nil), params.Libraries...)
	if len(libraries) == 0 && strings.TrimSpace(params.StoragePath) != "" {
		libraries = []Library{{Name: "默认资源库", Path: params.StoragePath}}
	}
	if strings.TrimSpace(params.StoragePath) == "" && len(libraries) > 0 {
		params.StoragePath = libraries[0].Path
	}

	user, err := newUser(params.Username, params.Password, UserRoleAdmin)
	if err != nil {
		return nil, err
	}

	jwtSecret, err := generateSecret(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT secret: %w", err)
	}

	cfg := &Config{
		Workshop:      Workshop{AppName: strings.TrimSpace(params.AppName)},
		Port:          params.Port,
		ActiveProfile: user.Username,
		StoragePath:   params.StoragePath,
		Libraries:     libraries,
		ThumbnailDir:  strings.TrimSpace(params.ThumbnailDir),
		ThumbnailSize: params.ThumbnailSize,
		TrashDir:      strings.TrimSpace(params.TrashDir),
		JWTSecret:     jwtSecret,
		Users:         []User{user},
		Preferences: Preferences{
			Theme:                         "light",
			GridSize:                      180,
			GridGap:                       2,
			ThumbRadius:                   2,
			SlideshowMode:                 "sequential",
			SlideshowLoop:                 true,
			SlideshowInterval:             5000,
			LightboxZoom:                  100,
			ExperimentalPrefetchNeighbors: true,
			ThrottledVideoSeek:            false,
			VideoSeekThrottleMS:           240,
			LowResourceMode:               false,
		},
	}
	if err := cfg.prepareRuntimePaths(); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// runInitWizard runs the command-line initialization wizard.
func runInitWizard() (*Config, error) {
	return runInitWizardWithReader(bufio.NewReader(os.Stdin), os.Stdout)
}

func runInitWizardWithReader(reader *bufio.Reader, out io.Writer) (*Config, error) {
	printf := func(format string, args ...any) {
		fmt.Fprintf(out, format, args...)
	}
	println := func(msg string) {
		fmt.Fprintln(out, msg)
	}
	println("No config file found. Starting initialization...")
	println("-----------------------------------")

	port, err := promptIntWithWriter(reader, out, "Server port", 8080, func(v int) error {
		if v <= 0 || v > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	storagePath, err := promptStringWithWriter(reader, out, "Media storage path", "./photos")
	if err != nil {
		return nil, err
	}

	storagePath = strings.TrimSpace(storagePath)
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}
	printf("Storage directory ready: %s\n", storagePath)

	println("")
	println("Create the default administrator account")
	println("-----------------------------------")
	username, err := promptStringWithWriter(reader, out, "Default username", "admin")
	if err != nil {
		return nil, err
	}
	var password string
	for {
		printf("Default password [at least 6 characters]: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read input: %w", err)
		}
		password = strings.TrimSpace(input)
		if len(password) < 6 {
			println("Invalid input: password must be at least 6 characters")
			continue
		}
		break
	}
	return CreateInitialConfig(InitialConfigParams{
		Port:        port,
		StoragePath: storagePath,
		Libraries: []Library{
			{Name: "默认资源库", Path: storagePath},
		},
		Username: username,
		Password: password,
	})
}

// promptString prompts for a string and supports a default value.
func promptString(reader *bufio.Reader, prompt, defaultVal string) (string, error) {
	return promptStringWithWriter(reader, os.Stdout, prompt, defaultVal)
}

func promptStringWithWriter(reader *bufio.Reader, out io.Writer, prompt, defaultVal string) (string, error) {
	if defaultVal == "" {
		fmt.Fprintf(out, "%s: ", prompt)
	} else {
		fmt.Fprintf(out, "%s [default: %s]: ", prompt, defaultVal)
	}
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal, nil
	}
	return input, nil
}

// promptInt prompts for an integer and supports a default value plus validation.
func promptInt(reader *bufio.Reader, prompt string, defaultVal int, validate func(int) error) (int, error) {
	return promptIntWithWriter(reader, os.Stdout, prompt, defaultVal, validate)
}

func promptIntWithWriter(reader *bufio.Reader, out io.Writer, prompt string, defaultVal int, validate func(int) error) (int, error) {
	for {
		fmt.Fprintf(out, "%s [default: %d]: ", prompt, defaultVal)
		input, err := reader.ReadString('\n')
		if err != nil {
			return 0, fmt.Errorf("failed to read input: %w", err)
		}
		input = strings.TrimSpace(input)
		if input == "" {
			return defaultVal, nil
		}
		v, err := strconv.Atoi(input)
		if err != nil {
			fmt.Fprintln(out, "Please enter a valid integer")
			continue
		}
		if validate != nil {
			if err := validate(v); err != nil {
				fmt.Fprintf(out, "Invalid input: %v\n", err)
				continue
			}
		}
		return v, nil
	}
}

// generateSecret 生成指定字节长度的随机十六进制字符串
func generateSecret(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
