module github.com/yyhuni/lunafox/engines/gogo_scan

go 1.26.0

require (
	github.com/yyhuni/lunafox/contracts v0.0.0
	github.com/yyhuni/lunafox/engine-go v0.0.0
	github.com/yyhuni/lunafox/engines v0.0.0
)

require (
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.38.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260523011958-0a33c5d7ca68 // indirect
	google.golang.org/grpc v1.82.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/yyhuni/lunafox/engine-go => ../../../engine-go

replace github.com/yyhuni/lunafox/engines => ..

replace github.com/yyhuni/lunafox/contracts => ../../../contracts
