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
		return errors.New("gogo execution dependencies are required")
	}
	config, enabled, err := projectConfig(snapshot.GetConfig())
	if err != nil {
		return err
	}
	if len(snapshot.GetConfigResources()) != 0 || len(snapshot.GetPlatformResources()) != 0 {
		return errors.New("gogo does not declare resources")
	}
	if !enabled {
		return reporter.Progress(ctx, "gogo scan disabled")
	}
	var subdomainsPath string
	if snapshot.GetTarget().GetType() == "domain" {
		subdomainsPath, err = inputSet.SubdomainsPath(ctx)
		if err != nil {
			return err
		}
	}
	if err := reporter.Progress(ctx, "gogo scan started"); err != nil {
		return err
	}
	_, err = chainreactor.RunGogo(ctx, "/opt/lunafox-tools/bin/gogo", chainreactor.GogoTarget{
		Type: snapshot.GetTarget().GetType(), Value: snapshot.GetTarget().GetValue(),
	}, subdomainsPath, protocol.WorkspacePath, config, func(item results.HostPort, _ *chainreactor.ServiceEndpoint) error {
		return reporter.Submit(ctx, item)
	})
	if err != nil {
		return err
	}
	return reporter.Progress(ctx, "gogo scan completed")
}

func projectConfig(config *protocol.EngineExecutionConfig) (chainreactor.GogoConfig, bool, error) {
	if config == nil || len(config.GetSections()) != 1 {
		return chainreactor.GogoConfig{}, false, errors.New("gogo config must contain one section")
	}
	section := config.GetSections()[0]
	if section == nil || section.GetSectionId() != "gogo" || section.Enabled == nil {
		return chainreactor.GogoConfig{}, false, errors.New("gogo config section is invalid")
	}
	if !section.GetEnabled() {
		if len(section.GetParams()) != 0 {
			return chainreactor.GogoConfig{}, false, errors.New("disabled gogo config has parameters")
		}
		return chainreactor.GogoConfig{}, false, nil
	}
	if len(section.GetParams()) != 4 {
		return chainreactor.GogoConfig{}, false, errors.New("gogo config requires four parameters")
	}
	values := make(map[string]*protocol.ConfigValue, 4)
	for _, value := range section.GetParams() {
		if value == nil || value.GetValue() == nil || values[value.GetParamKey()] != nil {
			return chainreactor.GogoConfig{}, false, errors.New("gogo config has duplicate or empty parameter")
		}
		values[value.GetParamKey()] = value
	}
	portValue := values["ports"]
	if portValue == nil {
		return chainreactor.GogoConfig{}, false, errors.New("gogo ports is missing")
	}
	ports, ok := portValue.GetValue().(*protocol.ConfigValue_StringValue)
	if !ok {
		return chainreactor.GogoConfig{}, false, errors.New("gogo ports type is invalid")
	}
	integer := func(key string) (int, error) {
		value := values[key]
		if value == nil {
			return 0, fmt.Errorf("gogo %s is missing", key)
		}
		typed, ok := value.GetValue().(*protocol.ConfigValue_IntegerValue)
		if !ok || typed.IntegerValue < 0 || typed.IntegerValue > 86400 {
			return 0, fmt.Errorf("gogo %s is invalid", key)
		}
		return int(typed.IntegerValue), nil
	}
	threads, err := integer("threads")
	if err != nil {
		return chainreactor.GogoConfig{}, false, err
	}
	socketTimeout, err := integer("socket-timeout")
	if err != nil {
		return chainreactor.GogoConfig{}, false, err
	}
	timeout, err := integer("timeout")
	if err != nil {
		return chainreactor.GogoConfig{}, false, err
	}
	return chainreactor.GogoConfig{
		Ports: ports.StringValue, Threads: threads,
		SocketTimeout: time.Duration(socketTimeout) * time.Second,
		Timeout:       time.Duration(timeout) * time.Second,
	}, true, nil
}
