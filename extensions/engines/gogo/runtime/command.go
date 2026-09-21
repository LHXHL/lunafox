package gogoruntime

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	enginecontract "github.com/yyhuni/lunafox/engines/gogo/contract"
)

// GogoBinary is the canonical runtime-image tool location.
const GogoBinary = "/opt/lunafox-tools/bin/gogo"

// BuildGogoArgs renders the gogo command line from the frozen engine config.
// Ports default to the port-tag preset; an explicit ports range wins when set.
// `-O jl -C` forces one plain JSONL record per line without deflate wrapping.
func BuildGogoArgs(config enginecontract.GogoConfig, candidatesPath, outputPath string) ([]string, error) {
	if !config.Enabled {
		return nil, errors.New("Gogo section must be enabled")
	}
	if candidatesPath == "" {
		return nil, errors.New("Gogo candidates path is required")
	}
	if outputPath == "" {
		return nil, errors.New("Gogo output path is required")
	}
	if err := validateGogoConfig(config); err != nil {
		return nil, err
	}
	ports := config.PortTag
	if config.Ports != "" {
		ports = config.Ports
	}
	args := []string{
		"--list", candidatesPath,
		"--port", ports,
		"--thread", strconv.FormatInt(config.Concurrency, 10),
		"--timeout", strconv.FormatInt(config.Timeout, 10),
		"--mod", config.Mod,
		"--file", outputPath,
		"--file-output", "jl",
		"--compress",
		"--quiet",
	}
	if config.Exploit {
		args = append(args, "--exploit")
	}
	if config.ActiveFinger {
		args = append(args, "--verbose")
	}
	return args, nil
}

func gogoProgressError(ctx context.Context, stage string, err error) error {
	if ctx != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
	}
	return fmt.Errorf("report Gogo %s progress: %w", stage, err)
}
