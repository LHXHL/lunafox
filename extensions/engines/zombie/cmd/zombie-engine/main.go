package main

import (
	"context"
	"fmt"

	enginecontract "github.com/yyhuni/lunafox/engines/zombie/contract"
	zombieruntime "github.com/yyhuni/lunafox/engines/zombie/runtime"
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
	return zombieruntime.New().Execute(ctx, execution)
}
