package main

import (
	"testing"
	"time"

	"github.com/yyhuni/lunafox/engine-go/protocol"
)

func TestProjectConfigRequiresBoundWordlistAndExactScalars(t *testing.T) {
	enabled := true
	valid := func() *protocol.EngineExecutionContext {
		return &protocol.EngineExecutionContext{
			Config: &protocol.EngineExecutionConfig{Sections: []*protocol.ConfigSection{{
				SectionId: "spray", Enabled: &enabled,
				Params: []*protocol.ConfigValue{
					{ParamKey: "pool", Value: &protocol.ConfigValue_IntegerValue{IntegerValue: 2}},
					{ParamKey: "threads-per-pool", Value: &protocol.ConfigValue_IntegerValue{IntegerValue: 10}},
					{ParamKey: "rate-per-pool", Value: &protocol.ConfigValue_IntegerValue{IntegerValue: 50}},
					{ParamKey: "request-timeout", Value: &protocol.ConfigValue_IntegerValue{IntegerValue: 10}},
					{ParamKey: "timeout", Value: &protocol.ConfigValue_IntegerValue{IntegerValue: 3600}},
				},
			}}},
			ConfigResources: []*protocol.ConfigResource{{
				SectionId: "spray", ParamKey: "wordlist", ContentType: "text/plain", Path: "/run/lunafox/resources/config/spray/wordlist/dir_default.txt",
			}},
		}
	}
	config, active, path, err := projectConfig(valid())
	if err != nil || !active || config.Pool != 2 || config.RequestTimeout != 10*time.Second || path == "" {
		t.Fatalf("project valid config = %+v, %t, %q, %v", config, active, path, err)
	}
	for name, mutate := range map[string]func(*protocol.EngineExecutionContext){
		"missing resource": func(value *protocol.EngineExecutionContext) { value.ConfigResources = nil },
		"wrong resource key": func(value *protocol.EngineExecutionContext) { value.ConfigResources[0].ParamKey = "password" },
		"unknown scalar": func(value *protocol.EngineExecutionContext) { value.Config.Sections[0].Params[0].ParamKey = "unknown" },
		"wrong scalar type": func(value *protocol.EngineExecutionContext) { value.Config.Sections[0].Params[0].Value = &protocol.ConfigValue_StringValue{StringValue: "2"} },
	} {
		t.Run(name, func(t *testing.T) {
			value := valid()
			mutate(value)
			if _, _, _, err := projectConfig(value); err == nil {
				t.Fatal("invalid Engine config was accepted")
			}
		})
	}
}
