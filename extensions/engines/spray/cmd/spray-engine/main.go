package main

import (
	"context"
	"fmt"

	enginecontract "github.com/yyhuni/lunafox/engines/spray/contract"
	sprayruntime "github.com/yyhuni/lunafox/engines/spray/runtime"
)

func main() {
	Run(runEngine)
}

func runEngine(ctx context.Context, execution *enginecontract.Execution) error {
	if ctx == nil {
		return fmt.Errorf("execution context is required")
	}
	if execution == nil {
		return fmt.Errorf("execution is required")
	}
	return sprayruntime.New().Execute(ctx, execution)
}
