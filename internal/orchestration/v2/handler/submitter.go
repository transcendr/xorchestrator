// Package handler provides command handlers for the v2 orchestration architecture.
//
// ProcessorSubmitterAdapter bridges WorkerProcess command submission to the FIFO processor.
// This adapter was moved from v2/worker package as part of the v2/worker deprecation effort.
package handler

import (
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
)

// ProcessorLike is the interface satisfied by CommandProcessor for submitting commands.
// This allows ProcessorSubmitterAdapter to work without depending on the processor package.
type ProcessorLike interface {
	Submit(cmd command.Command) error
}

// ProcessorSubmitterAdapter wraps a CommandProcessor to implement process.CommandSubmitter.
// It ignores the error from Submit() since WorkerProcess cannot meaningfully handle
// queue-full errors during turn completion - the command is fire-and-forget.
type ProcessorSubmitterAdapter struct {
	processor ProcessorLike
}

// NewProcessorSubmitterAdapter creates a new adapter wrapping the given processor.
func NewProcessorSubmitterAdapter(processor ProcessorLike) *ProcessorSubmitterAdapter {
	return &ProcessorSubmitterAdapter{processor: processor}
}

// Submit implements process.CommandSubmitter by delegating to the processor.
// Any error (e.g., queue full) is silently ignored as this is fire-and-forget.
func (a *ProcessorSubmitterAdapter) Submit(cmd command.Command) {
	if a.processor != nil {
		_ = a.processor.Submit(cmd)
	}
}

// Compile-time assertion: ProcessorSubmitterAdapter implements process.CommandSubmitter.
var _ process.CommandSubmitter = (*ProcessorSubmitterAdapter)(nil)
