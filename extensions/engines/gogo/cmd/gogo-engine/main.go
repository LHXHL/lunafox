package main

import (
	"context"
	"fmt"
	"os"

	enginecontract "github.com/yyhuni/lunafox/engines/gogo/contract"
	gogoruntime "github.com/yyhuni/lunafox/engines/gogo/runtime"
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
	if err := gogoruntime.New().Execute(ctx, execution); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gogo engine error: %v\n", err)
		return err
	}
	return nil
}
