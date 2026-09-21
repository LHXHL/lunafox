package sprayruntime

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	enginecontract "github.com/yyhuni/lunafox/engines/spray/contract"
)

// SprayBinary is the canonical runtime-image tool location.
const SprayBinary = "/opt/lunafox-tools/bin/spray"

// BuildSprayArgs renders the spray command line from the frozen engine
// config. Flags map 1:1 onto spray v0.3.x long options.
func BuildSprayArgs(config enginecontract.SprayConfig, candidatesPath, outputPath string) ([]string, error) {
	if !config.Enabled {
		return nil, errors.New("Spray section must be enabled")
	}
	if candidatesPath == "" {
		return nil, errors.New("Spray candidates path is required")
	}
	if outputPath == "" {
		return nil, errors.New("Spray output path is required")
	}
	if err := validateSprayConfigShallow(config); err != nil {
		return nil, err
	}
	args := []string{
		"--list", candidatesPath,
		"--dict", config.Wordlist,
		"--pool", strconv.FormatInt(config.Pool, 10),
		"--thread", strconv.FormatInt(config.Threads, 10),
		"--timeout", strconv.FormatInt(config.RequestTimeout, 10),
		"--mod", config.Mod,
		"--file", outputPath,
		"--file-output", "json",
		"--quiet",
	}
	return args, nil
}

func validateSprayConfigShallow(config enginecontract.SprayConfig) error {
	if config.Wordlist == "" {
		return errors.New("Spray wordlist resource is required")
	}
	if config.Pool < 1 || config.Pool > 50 {
		return errors.New("Spray pool must be between 1 and 50")
	}
	if config.Threads < 1 || config.Threads > 100 {
		return errors.New("Spray threads must be between 1 and 100")
	}
	if config.RequestTimeout < 1 || config.RequestTimeout > 120 {
		return errors.New("Spray request-timeout must be between 1 and 120")
	}
	if config.Mod != "path" && config.Mod != "host" {
		return errors.New("Spray mod must be path or host")
	}
	return nil
}

func sprayProgressError(ctx context.Context, stage string, err error) error {
	if ctx != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
	}
	return fmt.Errorf("report Spray %s progress: %w", stage, err)
}
