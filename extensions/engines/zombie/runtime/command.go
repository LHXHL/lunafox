package zombieruntime

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	enginecontract "github.com/yyhuni/lunafox/engines/zombie/contract"
)

// ZombieBinary is the canonical runtime-image tool location.
const ZombieBinary = "/opt/lunafox-tools/bin/zombie"

// BuildZombieArgs renders the zombie command line from the frozen engine
// config. Targets already carry `<service>://ip:port` scheme information, so
// one run covers every whitelisted service.
func BuildZombieArgs(config enginecontract.ZombieConfig, targetsPath, outputPath string) ([]string, error) {
	if !config.Enabled {
		return nil, errors.New("Zombie section must be enabled")
	}
	if targetsPath == "" {
		return nil, errors.New("Zombie targets path is required")
	}
	if outputPath == "" {
		return nil, errors.New("Zombie output path is required")
	}
	if err := validateZombieConfig(config); err != nil {
		return nil, err
	}
	args := []string{
		"--IP", targetsPath,
		"--thread", strconv.FormatInt(config.Threads, 10),
		"--file", outputPath,
		"--file-format", "json",
		"--quiet",
	}
	if config.Weakpass {
		args = append(args, "--weakpass", "--pwd", "admin")
	}
	return args, nil
}

func zombieProgressError(ctx context.Context, stage string, err error) error {
	if ctx != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
	}
	return fmt.Errorf("report Zombie %s progress: %w", stage, err)
}
