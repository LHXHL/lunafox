package main

import (
	"testing"
	"time"

	"github.com/yyhuni/lunafox/engine-go/protocol"
)

func TestProjectConfigRequiresExplicitCredentialBindings(t *testing.T) {
	enabled := true
	valid := func() *protocol.EngineExecutionContext {
		return &protocol.EngineExecutionContext{
			Config: &protocol.EngineExecutionConfig{Sections: []*protocol.ConfigSection{{
				SectionId: "zombie", Enabled: &enabled,
				Params: []*protocol.ConfigValue{
					{ParamKey: "timeout", Value: &protocol.ConfigValue_IntegerValue{IntegerValue: 900}},
					{ParamKey: "request-timeout", Value: &protocol.ConfigValue_IntegerValue{IntegerValue: 5}},
					{ParamKey: "max-attempts", Value: &protocol.ConfigValue_IntegerValue{IntegerValue: 100}},
					{ParamKey: "check-anonymous", Value: &protocol.ConfigValue_BooleanValue{BooleanValue: false}},
				},
			}}},
			ConfigResources: []*protocol.ConfigResource{
				{SectionId: "zombie", ParamKey: "usernames", ContentType: "application/vnd.lunafox.wordlist.v1", Path: "/run/lunafox/resources/config/zombie/usernames/zombie-test-users.txt"},
				{SectionId: "zombie", ParamKey: "passwords", ContentType: "application/vnd.lunafox.wordlist.v1", Path: "/run/lunafox/resources/config/zombie/passwords/zombie-test-passwords.txt"},
			},
		}
	}
	config, active, usernames, passwords, err := projectConfig(valid())
	if err != nil || !active || config.Timeout != 900*time.Second || config.CheckAnonymous || usernames == "" || passwords == "" {
		t.Fatalf("config=%+v active=%t usernames=%q passwords=%q err=%v", config, active, usernames, passwords, err)
	}
	for name, mutate := range map[string]func(*protocol.EngineExecutionContext){
		"missing passwords":   func(value *protocol.EngineExecutionContext) { value.ConfigResources = value.ConfigResources[:1] },
		"duplicate usernames": func(value *protocol.EngineExecutionContext) { value.ConfigResources[1].ParamKey = "usernames" },
		"wrong content type":  func(value *protocol.EngineExecutionContext) { value.ConfigResources[0].ContentType = "text/plain" },
		"wrong boolean type": func(value *protocol.EngineExecutionContext) {
			value.Config.Sections[0].Params[3].Value = &protocol.ConfigValue_StringValue{StringValue: "false"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := valid()
			mutate(value)
			if _, _, _, _, err := projectConfig(value); err == nil {
				t.Fatal("invalid zombie configuration was accepted")
			}
		})
	}
}
