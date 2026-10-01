package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/yyhuni/lunafox/contracts/results"
	"github.com/yyhuni/lunafox/engine-go/protocol"
	"github.com/yyhuni/lunafox/engines/chainreactor"
)

func main() { Run(run) }

func run(ctx context.Context, snapshot *protocol.EngineExecutionContext, reporter *engineReportPort, inputSet engineInputs) error {
	if snapshot == nil || snapshot.GetTarget() == nil || reporter == nil || inputSet == nil {
		return errors.New("spray execution dependencies are required")
	}
	config, enabled, wordlistPath, err := projectConfig(snapshot)
	if err != nil {
		return err
	}
	if len(snapshot.GetPlatformResources()) != 0 {
		return errors.New("spray does not declare platform resources")
	}
	if !enabled {
		return reporter.Progress(ctx, "spray scan disabled")
	}
	websiteURLsPath, err := inputSet.WebsiteURLsPath(ctx)
	if err != nil {
		return err
	}
	if err := reporter.Progress(ctx, "spray scan started"); err != nil {
		return err
	}
	_, err = chainreactor.RunSpray(ctx, "/opt/lunafox-tools/bin/spray", websiteURLsPath, wordlistPath, protocol.WorkspacePath, config, func(item results.Directory) error {
		return reporter.Submit(ctx, item)
	})
	if err != nil {
		return err
	}
	return reporter.Progress(ctx, "spray scan completed")
}

func projectConfig(snapshot *protocol.EngineExecutionContext) (chainreactor.SprayConfig, bool, string, error) {
	config := snapshot.GetConfig()
	if config == nil || len(config.GetSections()) != 1 {
		return chainreactor.SprayConfig{}, false, "", errors.New("spray config must contain one section")
	}
	section := config.GetSections()[0]
	if section == nil || section.GetSectionId() != "spray" || section.Enabled == nil {
		return chainreactor.SprayConfig{}, false, "", errors.New("spray config section is invalid")
	}
	if !section.GetEnabled() {
		if len(section.GetParams()) != 0 || len(snapshot.GetConfigResources()) != 0 {
			return chainreactor.SprayConfig{}, false, "", errors.New("disabled spray config contains parameters or resources")
		}
		return chainreactor.SprayConfig{}, false, "", nil
	}
	if len(section.GetParams()) != 5 || len(snapshot.GetConfigResources()) != 1 {
		return chainreactor.SprayConfig{}, false, "", errors.New("spray config parameters or wordlist binding are incomplete")
	}
	resource := snapshot.GetConfigResources()[0]
	if resource == nil || resource.GetSectionId() != "spray" || resource.GetParamKey() != "wordlist" || resource.GetContentType() == "" || resource.GetPath() == "" {
		return chainreactor.SprayConfig{}, false, "", errors.New("spray wordlist binding is invalid")
	}
	values := make(map[string]*protocol.ConfigValue, 5)
	for _, value := range section.GetParams() {
		if value == nil || value.GetValue() == nil || values[value.GetParamKey()] != nil {
			return chainreactor.SprayConfig{}, false, "", errors.New("spray config contains a duplicate or empty parameter")
		}
		values[value.GetParamKey()] = value
	}
	integer := func(key string) (int, error) {
		value := values[key]
		if value == nil {
			return 0, fmt.Errorf("spray %s is missing", key)
		}
		typed, ok := value.GetValue().(*protocol.ConfigValue_IntegerValue)
		if !ok || typed.IntegerValue < 0 || typed.IntegerValue > 86400 {
			return 0, fmt.Errorf("spray %s is invalid", key)
		}
		return int(typed.IntegerValue), nil
	}
	pool, err := integer("pool")
	if err != nil {
		return chainreactor.SprayConfig{}, false, "", err
	}
	threads, err := integer("threads-per-pool")
	if err != nil {
		return chainreactor.SprayConfig{}, false, "", err
	}
	rate, err := integer("rate-per-pool")
	if err != nil {
		return chainreactor.SprayConfig{}, false, "", err
	}
	requestTimeout, err := integer("request-timeout")
	if err != nil {
		return chainreactor.SprayConfig{}, false, "", err
	}
	timeout, err := integer("timeout")
	if err != nil {
		return chainreactor.SprayConfig{}, false, "", err
	}
	return chainreactor.SprayConfig{
		Pool: pool, ThreadsPerPool: threads, RatePerPool: rate,
		RequestTimeout: time.Duration(requestTimeout) * time.Second,
		Timeout:        time.Duration(timeout) * time.Second,
	}, true, resource.GetPath(), nil
}
