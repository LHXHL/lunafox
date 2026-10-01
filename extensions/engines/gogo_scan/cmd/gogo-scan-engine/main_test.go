package main

import (
	"testing"
	"time"

	"github.com/yyhuni/lunafox/engine-go/protocol"
)

func TestProjectConfigRequiresExactManifestShape(t *testing.T) {
	enabled := true
	valid := func() *protocol.EngineExecutionConfig {
		return &protocol.EngineExecutionConfig{Sections: []*protocol.ConfigSection{{
			SectionId: "gogo", Enabled: &enabled,
			Params: []*protocol.ConfigValue{
				{ParamKey: "ports", Value: &protocol.ConfigValue_StringValue{StringValue: "top1"}},
				{ParamKey: "threads", Value: &protocol.ConfigValue_IntegerValue{IntegerValue: 100}},
				{ParamKey: "socket-timeout", Value: &protocol.ConfigValue_IntegerValue{IntegerValue: 2}},
				{ParamKey: "timeout", Value: &protocol.ConfigValue_IntegerValue{IntegerValue: 14400}},
			},
		}}}
	}
	config, active, err := projectConfig(valid())
	if err != nil || !active || config.Threads != 100 || config.SocketTimeout != 2*time.Second {
		t.Fatalf("project valid config = %+v, %t, %v", config, active, err)
	}
	for name, mutate := range map[string]func(*protocol.EngineExecutionConfig){
		"missing ports": func(value *protocol.EngineExecutionConfig) { value.Sections[0].Params[0].ParamKey = "unknown" },
		"wrong type": func(value *protocol.EngineExecutionConfig) { value.Sections[0].Params[1].Value = &protocol.ConfigValue_StringValue{StringValue: "100"} },
		"duplicate": func(value *protocol.EngineExecutionConfig) { value.Sections[0].Params[1].ParamKey = "ports" },
		"extra section": func(value *protocol.EngineExecutionConfig) { value.Sections = append(value.Sections, &protocol.ConfigSection{SectionId: "other"}) },
	} {
		t.Run(name, func(t *testing.T) {
			value := valid()
			mutate(value)
			if _, _, err := projectConfig(value); err == nil {
				t.Fatal("invalid Engine config was accepted")
			}
		})
	}
}
