//go:build mcp && !darwin && !ios && !android

package application

import (
	"fmt"
	"runtime"
)

func mcpSendNativeMouseMove(window Window, deltaX, deltaY float64) error {
	return fmt.Errorf("native mouse movement is not implemented for %s", runtime.GOOS)
}
