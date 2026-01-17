// Package types provides shared interfaces for the v2 orchestration architecture.
// This package exists to avoid import cycles between processor and handler packages.
package types

import (
	"context"

	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/command"
)

// CommandHandler processes a command and returns a result.
// Handlers are executed sequentially by the processor - never concurrently.
type CommandHandler interface {
	Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error)
}

// HandlerFunc is an adapter to allow ordinary functions to be used as CommandHandlers.
type HandlerFunc func(ctx context.Context, cmd command.Command) (*command.CommandResult, error)

// Handle calls f(ctx, cmd).
func (f HandlerFunc) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	return f(ctx, cmd)
}
