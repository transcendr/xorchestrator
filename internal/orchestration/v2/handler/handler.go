// Package handler provides the CommandHandler interface, HandlerRegistry,
// and result builder helpers for the v2 orchestration architecture.
package handler

import (
	"context"
	"fmt"
	"sync"

	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/command"
	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/types"
)

// CommandHandler is an alias for types.CommandHandler.
// See types.CommandHandler for documentation.
type CommandHandler = types.CommandHandler

// HandlerFunc is an alias for types.HandlerFunc.
// See types.HandlerFunc for documentation.
type HandlerFunc = types.HandlerFunc

// ===========================================================================
// Message Delivery Interface
// ===========================================================================

// MessageDeliverer is responsible for delivering messages to processes.
// Implementations handle the actual delivery mechanism (e.g., AI resume, MCP callback).
type MessageDeliverer interface {
	// Deliver sends a message to a process.
	// Returns an error if delivery fails.
	Deliver(ctx context.Context, processID, content string) error
}

// ===========================================================================
// Handler Registry
// ===========================================================================

// ErrHandlerNotFound is returned when a handler is not registered for a command type.
var ErrHandlerNotFound = types.ErrHandlerNotFound

// HandlerRegistry provides type-safe registration and lookup of command handlers.
// It maps command types to their respective handlers.
type HandlerRegistry struct {
	mu       sync.RWMutex
	handlers map[command.CommandType]CommandHandler
}

// NewHandlerRegistry creates a new empty HandlerRegistry.
func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		handlers: make(map[command.CommandType]CommandHandler),
	}
}

// Register registers a handler for a command type.
// If a handler is already registered for the command type, it will be replaced.
func (r *HandlerRegistry) Register(cmdType command.CommandType, handler CommandHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[cmdType] = handler
}

// Get returns the handler registered for the given command type.
// Returns ErrHandlerNotFound if no handler is registered.
func (r *HandlerRegistry) Get(cmdType command.CommandType) (CommandHandler, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	handler, ok := r.handlers[cmdType]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrHandlerNotFound, cmdType)
	}
	return handler, nil
}

// MustGet returns the handler registered for the given command type.
// Panics if no handler is registered.
func (r *HandlerRegistry) MustGet(cmdType command.CommandType) CommandHandler {
	handler, err := r.Get(cmdType)
	if err != nil {
		panic(fmt.Sprintf("handler not registered: %s", cmdType))
	}
	return handler
}

// RegisteredTypes returns a slice of all registered command types.
// This is useful for debugging and testing.
func (r *HandlerRegistry) RegisteredTypes() []command.CommandType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]command.CommandType, 0, len(r.handlers))
	for cmdType := range r.handlers {
		types = append(types, cmdType)
	}
	return types
}

// Route routes a command to the appropriate handler and executes it.
// Returns ErrHandlerNotFound if no handler is registered for the command type.
func (r *HandlerRegistry) Route(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	handler, err := r.Get(cmd.Type())
	if err != nil {
		return nil, err
	}
	return handler.Handle(ctx, cmd)
}

// ===========================================================================
// Result Builder Helpers
// ===========================================================================

// SuccessResult creates a successful CommandResult with optional data.
func SuccessResult(data any) *command.CommandResult {
	return &command.CommandResult{
		Success: true,
		Data:    data,
	}
}

// SuccessWithEvents creates a successful CommandResult with events to emit.
func SuccessWithEvents(data any, events ...any) *command.CommandResult {
	return &command.CommandResult{
		Success: true,
		Data:    data,
		Events:  events,
	}
}

// SuccessWithFollowUp creates a successful CommandResult with follow-up commands.
func SuccessWithFollowUp(data any, followUp ...command.Command) *command.CommandResult {
	return &command.CommandResult{
		Success:  true,
		Data:     data,
		FollowUp: followUp,
	}
}

// SuccessWithEventsAndFollowUp creates a successful CommandResult with both events and follow-up commands.
func SuccessWithEventsAndFollowUp(data any, events []any, followUp []command.Command) *command.CommandResult {
	return &command.CommandResult{
		Success:  true,
		Data:     data,
		Events:   events,
		FollowUp: followUp,
	}
}

// ErrorResult creates a failed CommandResult with an error.
func ErrorResult(err error) *command.CommandResult {
	return &command.CommandResult{
		Success: false,
		Error:   err,
	}
}
