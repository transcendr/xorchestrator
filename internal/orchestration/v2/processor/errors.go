// Package processor provides the FIFO command processor for the v2 orchestration architecture.
// This file re-exports error sentinels from the types package for backward compatibility.
package processor

import (
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/types"
)

// ErrUnknownCommandType is returned when no handler is registered for a command type.
var ErrUnknownCommandType = types.ErrUnknownCommandType

// ErrProcessorNotRunning is returned when submitting to a stopped processor.
var ErrProcessorNotRunning = types.ErrProcessorNotRunning

// CommandErrorEvent is emitted when a command fails validation or handler execution.
type CommandErrorEvent struct {
	CommandID   string
	CommandType command.CommandType
	Error       error
}
