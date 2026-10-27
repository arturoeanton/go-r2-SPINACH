package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// LoadEnv reads configuration as data, never as shell code. Existing environment wins.
func LoadEnv(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	values := map[string]string{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	line := 0
	for scanner.Scan() {
		line++
		s := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		s = strings.TrimPrefix(s, "export ")
		key, value, ok := strings.Cut(s, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || !validKey(key) {
			return fmt.Errorf("invalid .env assignment at line %d", line)
		}
		if strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'") {
			quote := value[0]
			end := -1
			for i := 1; i < len(value); i++ {
				if value[i] == quote && (quote == '\'' || value[i-1] != '\\') {
					end = i
					break
				}
			}
			if end < 0 || strings.TrimSpace(value[end+1:]) != "" && !strings.HasPrefix(strings.TrimSpace(value[end+1:]), "#") {
				return fmt.Errorf("invalid .env quoting at line %d", line)
			}
			if quote == '\'' {
				value = value[1:end]
			} else {
				value, err = strconv.Unquote(value[:end+1])
				if err != nil {
					return fmt.Errorf("invalid .env quoting at line %d", line)
				}
			}
		} else {
			for i, c := range value {
				if c == '#' && (i == 0 || value[i-1] == ' ' || value[i-1] == '\t') {
					value = strings.TrimSpace(value[:i])
					break
				}
			}
		}
		values[key] = value
	}
	if err = scanner.Err(); err != nil {
		return errors.New("could not read .env")
	}
	for key, value := range values {
		if _, exists := os.LookupEnv(key); !exists {
			if err = os.Setenv(key, value); err != nil {
				return fmt.Errorf("could not set environment variable %s", key)
			}
		}
	}
	return nil
}
func validKey(key string) bool {
	if key == "" {
		return false
	}
	for i, c := range key {
		if c != '_' && !(c >= 'A' && c <= 'Z') && !(c >= 'a' && c <= 'z') && !(i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}
