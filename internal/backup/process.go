package backup

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Keep passwords out of command arguments and subprocess error messages.
func postgresConnection(databaseURL string) (string, []string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", nil, errors.New("invalid PostgreSQL connection URL")
	}
	password, hasPassword := "", false
	if parsed.User != nil {
		password, hasPassword = parsed.User.Password()
		parsed.User = url.User(parsed.User.Username())
	}
	query := parsed.Query()
	if values, ok := query["password"]; ok {
		if len(values) != 1 {
			return "", nil, errors.New("invalid PostgreSQL password parameter")
		}
		password, hasPassword = values[0], true
		query.Del("password")
		parsed.RawQuery = query.Encode()
	}
	env := os.Environ()
	if hasPassword {
		filtered := make([]string, 0, len(env)+1)
		for _, item := range env {
			if !strings.HasPrefix(strings.ToUpper(item), "PGPASSWORD=") {
				filtered = append(filtered, item)
			}
		}
		env = append(filtered, "PGPASSWORD="+password)
	}
	return parsed.String(), env, nil
}

func pgCommand(ctx context.Context, name, databaseURL string, args ...string) (*exec.Cmd, error) {
	connection, env, err := postgresConnection(databaseURL)
	if err != nil {
		return nil, err
	}
	args = append(args, "--dbname="+connection)
	command := name
	if name == "pg_dump" && runtime.GOOS == "linux" {
		args = append([]string{"-n", "10", name}, args...)
		command = "nice"
	}
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = env
	return cmd, nil
}

func runPG(cmd *exec.Cmd, operation string) error {
	if err := cmd.Run(); err != nil {
		if cmd.Err != nil {
			return fmt.Errorf("%s executable unavailable: %w", operation, cmd.Err)
		}
		return fmt.Errorf("%s failed: %w", operation, err)
	}
	return nil
}

func redactError(err error, databaseURL string) string {
	message := strings.ReplaceAll(err.Error(), databaseURL, "[REDACTED]")
	if parsed, parseErr := url.Parse(databaseURL); parseErr == nil {
		var passwords []string
		if parsed.User != nil {
			if password, ok := parsed.User.Password(); ok {
				passwords = append(passwords, password)
			}
		}
		passwords = append(passwords, parsed.Query().Get("password"))
		for _, password := range passwords {
			if password != "" {
				message = strings.ReplaceAll(message, password, "[REDACTED]")
				message = strings.ReplaceAll(message, url.QueryEscape(password), "[REDACTED]")
			}
		}
	}
	return message
}
