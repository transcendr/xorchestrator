// Package events defines typed event structures for the orchestration layer.
// These events are published via the pubsub broker and consumed by the TUI
// and other subscribers.
//
// Event types:
//   - ProcessEvent: Unified events from both coordinator and worker processes
//   - MCPEvent: MCP protocol events (tool calls, results)
//
// The ProcessEvent type uses the Role field to distinguish between coordinator
// (RoleCoordinator) and worker (RoleWorker) events. This unified type replaces
// the legacy separate CoordinatorEvent and WorkerEvent types.
package events
