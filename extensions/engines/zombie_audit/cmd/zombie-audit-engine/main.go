package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/yyhuni/lunafox/engine-go/protocol"
	"github.com/yyhuni/lunafox/engines/chainreactor"
)

func main() { Run(run) }

func run(ctx context.Context, snapshot *protocol.EngineExecutionContext, reporter *engineReportPort, inputSet engineInputs) error {
	if snapshot == nil || snapshot.GetTarget() == nil || reporter == nil || inputSet == nil {
		return errors.New("zombie execution dependencies are required")
	}
	config, enabled, usernamesPath, passwordsPath, err := projectConfig(snapshot)
	if err != nil {
		return err
	}
	if len(snapshot.GetPlatformResources()) != 0 {
		return errors.New("zombie does not declare platform resources")
	}
	if !enabled {
		return reporter.Progress(ctx, "zombie audit disabled")
	}
	websiteURLsPath, err := inputSet.WebsiteURLsPath(ctx)
	if err != nil {
		return err
	}
	endpoints, err := chainreactor.ZombieWebTargets(ctx, websiteURLsPath)
	if err != nil {
		return err
	}
	if err := reporter.Progress(ctx, "zombie Web audit started"); err != nil {
		return err
	}
	_, err = chainreactor.RunZombieLocalVerification(ctx, "/opt/lunafox-tools/bin/zombie", endpoints, usernamesPath, passwordsPath, protocol.WorkspacePath, config, func(finding chainreactor.AuthFinding) error {
		item, err := chainreactor.ZombieAuthResult(finding)
		if err != nil {
			return err
		}
		return reporter.Submit(ctx, item)
	})
	if err != nil {
		return err
	}
	return reporter.Progress(ctx, "zombie Web audit completed")
}

func projectConfig(snapshot *protocol.EngineExecutionContext) (chainreactor.ZombieConfig, bool, string, string, error) {
	config := snapshot.GetConfig()
	if config == nil || len(config.GetSections()) != 1 {
		return chainreactor.ZombieConfig{}, false, "", "", errors.New("zombie config must contain one section")
	}
	section := config.GetSections()[0]
	if section == nil || section.GetSectionId() != "zombie" || section.Enabled == nil {
		return chainreactor.ZombieConfig{}, false, "", "", errors.New("zombie config section is invalid")
	}
	if !section.GetEnabled() {
		if len(section.GetParams()) != 0 || len(snapshot.GetConfigResources()) != 0 {
			return chainreactor.ZombieConfig{}, false, "", "", errors.New("disabled zombie config contains parameters or resources")
		}
		return chainreactor.ZombieConfig{}, false, "", "", nil
	}
	if len(section.GetParams()) != 4 || len(snapshot.GetConfigResources()) != 2 {
		return chainreactor.ZombieConfig{}, false, "", "", errors.New("zombie config parameters or test credential bindings are incomplete")
	}
	bindings := make(map[string]string, 2)
	for _, resource := range snapshot.GetConfigResources() {
		if resource == nil || resource.GetSectionId() != "zombie" || resource.GetContentType() != "application/vnd.lunafox.wordlist.v1" || resource.GetPath() == "" || bindings[resource.GetParamKey()] != "" {
			return chainreactor.ZombieConfig{}, false, "", "", errors.New("zombie test credential binding is invalid")
		}
		bindings[resource.GetParamKey()] = resource.GetPath()
	}
	if bindings["usernames"] == "" || bindings["passwords"] == "" || len(bindings) != 2 {
		return chainreactor.ZombieConfig{}, false, "", "", errors.New("zombie test credential bindings are incomplete")
	}
	values := make(map[string]*protocol.ConfigValue, 4)
	for _, value := range section.GetParams() {
		if value == nil || value.GetValue() == nil || values[value.GetParamKey()] != nil {
			return chainreactor.ZombieConfig{}, false, "", "", errors.New("zombie config contains a duplicate or empty parameter")
		}
		values[value.GetParamKey()] = value
	}
	integer := func(key string) (int, error) {
		value := values[key]
		if value == nil {
			return 0, fmt.Errorf("zombie %s is missing", key)
		}
		typed, ok := value.GetValue().(*protocol.ConfigValue_IntegerValue)
		if !ok || typed.IntegerValue < 0 || typed.IntegerValue > 3600 {
			return 0, fmt.Errorf("zombie %s is invalid", key)
		}
		return int(typed.IntegerValue), nil
	}
	timeout, err := integer("timeout")
	if err != nil {
		return chainreactor.ZombieConfig{}, false, "", "", err
	}
	requestTimeout, err := integer("request-timeout")
	if err != nil {
		return chainreactor.ZombieConfig{}, false, "", "", err
	}
	maxAttempts, err := integer("max-attempts")
	if err != nil {
		return chainreactor.ZombieConfig{}, false, "", "", err
	}
	check, ok := values["check-anonymous"]
	if !ok || check == nil {
		return chainreactor.ZombieConfig{}, false, "", "", errors.New("zombie check-anonymous is missing")
	}
	boolean, ok := check.GetValue().(*protocol.ConfigValue_BooleanValue)
	if !ok {
		return chainreactor.ZombieConfig{}, false, "", "", errors.New("zombie check-anonymous is invalid")
	}
	result := chainreactor.ZombieConfig{Timeout: time.Duration(timeout) * time.Second, RequestTimeout: time.Duration(requestTimeout) * time.Second, MaxAttempts: maxAttempts, CheckAnonymous: boolean.BooleanValue}
	return result, true, bindings["usernames"], bindings["passwords"], nil
}
