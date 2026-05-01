package profiles

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

type Profile struct {
	Name         string
	Endpoint     string
	Model        string
	Runtime      string
	Variant      string
	Quantization string
	GPU          string
	Concurrency  int
	Requests     int
	Timeout      time.Duration
	SoakMinutes  int
}

func Parse(r io.Reader) (Profile, error) {
	values, err := parseKeyValues(r)
	if err != nil {
		return Profile{}, err
	}

	profile := Profile{
		Name:         values["name"],
		Endpoint:     values["endpoint"],
		Model:        values["model"],
		Runtime:      values["runtime"],
		Variant:      values["variant"],
		Quantization: values["quantization"],
		GPU:          values["gpu"],
	}

	if profile.Name == "" || profile.Endpoint == "" || profile.Model == "" {
		return Profile{}, fmt.Errorf("profile requires name, endpoint, and model")
	}

	profile.Concurrency, err = positiveInt(values["concurrency"], "concurrency")
	if err != nil {
		return Profile{}, err
	}
	profile.Requests, err = positiveInt(values["requests"], "requests")
	if err != nil {
		return Profile{}, err
	}

	timeout, err := time.ParseDuration(values["timeout"])
	if err != nil || timeout <= 0 {
		return Profile{}, fmt.Errorf("timeout must be a positive duration")
	}
	profile.Timeout = timeout
	if values["soak_minutes"] != "" {
		profile.SoakMinutes, err = positiveInt(values["soak_minutes"], "soak_minutes")
		if err != nil {
			return Profile{}, err
		}
	}

	return profile, nil
}

func parseKeyValues(r io.Reader) (map[string]string, error) {
	scanner := bufio.NewScanner(r)
	values := make(map[string]string)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid profile line %q", line)
		}

		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}

	return values, nil
}

func positiveInt(value, name string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}

	return parsed, nil
}
