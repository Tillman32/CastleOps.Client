package api

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/castleops/client/internal/cache"
	"github.com/rs/zerolog"
)

// CommandHandler manages async command execution
// It receives commands from the server, executes them asynchronously,
// and reports results back to the server
type CommandHandler struct {
	// client is the API client for submitting results
	client *Client

	// cache stores pending commands for persistence
	cache cache.Cache

	// logger for structured logging
	logger zerolog.Logger

	// executors maps command types to execution functions
	executors map[CommandType]CommandExecutor

	// workerPool limits concurrent command execution
	workerPool chan struct{}

	// mu protects executors map
	mu sync.RWMutex

	// wg tracks running command executions
	wg sync.WaitGroup

	// ctx is the handler's context for cancellation
	ctx context.Context

	// cancel stops the handler
	cancel context.CancelFunc
}

// CommandExecutor is a function that executes a specific command type
// It receives the command payload and returns execution result
type CommandExecutor func(ctx context.Context, payload interface{}) (*CommandResult, error)

// CommandResult contains the outcome of command execution
type CommandResult struct {
	// Status indicates success or failure
	Status string

	// Output contains stdout/stderr
	Output string

	// Error contains error message if failed
	Error string

	// ExecutionTime is duration in milliseconds
	ExecutionTime int64
}

// CommandHandlerConfig configures the command handler
type CommandHandlerConfig struct {
	// Client is the API client
	Client *Client

	// Cache stores pending commands
	Cache cache.Cache

	// Logger for structured logging
	Logger zerolog.Logger

	// MaxConcurrentCommands limits parallel execution (default: 5)
	MaxConcurrentCommands int
}

// NewCommandHandler creates a new command handler
func NewCommandHandler(config CommandHandlerConfig) *CommandHandler {
	if config.MaxConcurrentCommands <= 0 {
		config.MaxConcurrentCommands = 5
	}

	ctx, cancel := context.WithCancel(context.Background())

	handler := &CommandHandler{
		client:     config.Client,
		cache:      config.Cache,
		logger:     config.Logger,
		executors:  make(map[CommandType]CommandExecutor),
		workerPool: make(chan struct{}, config.MaxConcurrentCommands),
		ctx:        ctx,
		cancel:     cancel,
	}

	return handler
}

// RegisterExecutor registers a command executor for a specific command type
// This allows extensible command handling
func (h *CommandHandler) RegisterExecutor(cmdType CommandType, executor CommandExecutor) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.executors[cmdType] = executor
	h.logger.Debug().
		Str("command_type", string(cmdType)).
		Msg("Registered command executor")
}

// HandleCommand processes a single command asynchronously
// It stores the command, executes it, and reports results
func (h *CommandHandler) HandleCommand(cmd *Command) error {
	// Store command in cache for persistence
	cacheCmd := &cache.Command{
		CommandID: cmd.CommandID,
		Type:      string(cmd.Type),
		Status:    cache.StatusPending,
		CreatedAt: time.Now().UTC(),
	}

	// Serialize payload
	payloadBytes, err := json.Marshal(cmd.Payload)
	if err != nil {
		return fmt.Errorf("failed to serialize command payload: %w", err)
	}
	cacheCmd.Payload = string(payloadBytes)

	// Store in cache
	if err := h.cache.StoreCommand(context.Background(), cacheCmd); err != nil {
		h.logger.Error().
			Err(err).
			Str("command_id", cmd.CommandID).
			Msg("Failed to cache command")
		return err
	}

	h.logger.Info().
		Str("command_id", cmd.CommandID).
		Str("type", string(cmd.Type)).
		Msg("Command queued for execution")

	// Execute asynchronously
	h.wg.Add(1)
	go h.executeCommand(cmd)

	return nil
}

// HandleCommands processes multiple commands
func (h *CommandHandler) HandleCommands(commands []*Command) error {
	var firstErr error

	for _, cmd := range commands {
		if err := h.HandleCommand(cmd); err != nil {
			h.logger.Error().
				Err(err).
				Str("command_id", cmd.CommandID).
				Msg("Failed to handle command")
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

// executeCommand runs a command with timeout and reports results
func (h *CommandHandler) executeCommand(cmd *Command) {
	defer h.wg.Done()

	// Acquire worker slot (blocks if pool is full)
	select {
	case h.workerPool <- struct{}{}:
		defer func() { <-h.workerPool }()
	case <-h.ctx.Done():
		h.logger.Warn().
			Str("command_id", cmd.CommandID).
			Msg("Command execution cancelled (handler stopped)")
		return
	}

	h.logger.Info().
		Str("command_id", cmd.CommandID).
		Str("type", string(cmd.Type)).
		Msg("Executing command")

	startTime := time.Now()

	// Create execution context with timeout
	execCtx := h.ctx
	if cmd.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(h.ctx, time.Duration(cmd.Timeout)*time.Second)
		defer cancel()
	}

	// Find executor for this command type
	h.mu.RLock()
	executor, exists := h.executors[cmd.Type]
	h.mu.RUnlock()

	var result *CommandResult
	var err error

	if !exists {
		// No executor registered for this command type
		result = &CommandResult{
			Status:        CommandStatusFailed,
			Error:         fmt.Sprintf("no executor registered for command type: %s", cmd.Type),
			ExecutionTime: time.Since(startTime).Milliseconds(),
		}
	} else {
		// Execute the command
		result, err = executor(execCtx, cmd.Payload)
		if err != nil {
			h.logger.Error().
				Err(err).
				Str("command_id", cmd.CommandID).
				Msg("Command execution failed")

			result = &CommandResult{
				Status:        CommandStatusFailed,
				Error:         err.Error(),
				ExecutionTime: time.Since(startTime).Milliseconds(),
			}
		} else if result == nil {
			// Executor didn't return a result
			result = &CommandResult{
				Status:        CommandStatusSuccess,
				ExecutionTime: time.Since(startTime).Milliseconds(),
			}
		}
	}

	// Ensure execution time is set
	if result.ExecutionTime == 0 {
		result.ExecutionTime = time.Since(startTime).Milliseconds()
	}

	// Submit result to server
	h.submitResult(cmd.CommandID, result)

	// Mark command as complete in cache
	if err := h.cache.MarkCommandComplete(context.Background(), cmd.CommandID); err != nil {
		h.logger.Error().
			Err(err).
			Str("command_id", cmd.CommandID).
			Msg("Failed to mark command complete in cache")
	}

	h.logger.Info().
		Str("command_id", cmd.CommandID).
		Str("status", result.Status).
		Int64("execution_time_ms", result.ExecutionTime).
		Msg("Command execution completed")
}

// submitResult sends command execution results to the server
// Retries on failure to ensure results are delivered
func (h *CommandHandler) submitResult(commandID string, result *CommandResult) {
	req := &CommandResultRequest{
		CommandID:     commandID,
		Status:        result.Status,
		Output:        result.Output,
		Error:         result.Error,
		ExecutionTime: result.ExecutionTime,
		CompletedAt:   time.Now().UTC(),
	}

	// Use a background context with timeout for result submission
	// This ensures we don't block even if handler is stopping
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := h.client.SubmitCommandResult(ctx, commandID, req)
	if err != nil {
		h.logger.Error().
			Err(err).
			Str("command_id", commandID).
			Msg("Failed to submit command result to server")
		// TODO: Store failed result submissions in cache for retry
		return
	}

	h.logger.Debug().
		Str("command_id", commandID).
		Msg("Command result submitted to server")
}

// ProcessPendingCommands loads and processes commands from cache
// This is useful for resuming after a restart
func (h *CommandHandler) ProcessPendingCommands(ctx context.Context) error {
	pending, err := h.cache.GetPendingCommands(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending commands: %w", err)
	}

	if len(pending) == 0 {
		h.logger.Debug().Msg("No pending commands in cache")
		return nil
	}

	h.logger.Info().
		Int("count", len(pending)).
		Msg("Processing pending commands from cache")

	for _, cacheCmd := range pending {
		// Convert cache command to API command
		var payload interface{}
		if err := json.Unmarshal([]byte(cacheCmd.Payload), &payload); err != nil {
			h.logger.Error().
				Err(err).
				Str("command_id", cacheCmd.CommandID).
				Msg("Failed to unmarshal command payload")
			continue
		}

		cmd := &Command{
			CommandID: cacheCmd.CommandID,
			Type:      CommandType(cacheCmd.Type),
			Payload:   payload,
		}

		// Execute the command (already in cache, so skip caching step)
		h.wg.Add(1)
		go h.executeCommand(cmd)
	}

	return nil
}

// Stop gracefully shuts down the command handler
// It waits for all running commands to complete
func (h *CommandHandler) Stop(timeout time.Duration) error {
	h.logger.Info().Msg("Stopping command handler")

	// Signal all goroutines to stop
	h.cancel()

	// Wait for running commands with timeout
	done := make(chan struct{})
	go func() {
		h.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		h.logger.Info().Msg("All commands completed")
		return nil
	case <-time.After(timeout):
		h.logger.Warn().
			Dur("timeout", timeout).
			Msg("Command handler stop timeout, some commands may not have completed")
		return fmt.Errorf("timeout waiting for commands to complete")
	}
}

// parseCommandPayload parses a command payload into the expected type
// This is a helper for executors
func parseCommandPayload(payload interface{}, target interface{}) error {
	// Re-marshal and unmarshal to convert interface{} to specific type
	// This is necessary because JSON unmarshaling into interface{} uses map[string]interface{}
	bytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	if err := json.Unmarshal(bytes, target); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	return nil
}

// InstallPackageExecutor creates an executor for package installation
func InstallPackageExecutor(installFunc func(ctx context.Context, payload *InstallPackagePayload) (string, error)) CommandExecutor {
	return func(ctx context.Context, payload interface{}) (*CommandResult, error) {
		var p InstallPackagePayload
		if err := parseCommandPayload(payload, &p); err != nil {
			return nil, err
		}

		output, err := installFunc(ctx, &p)
		if err != nil {
			return &CommandResult{
				Status: CommandStatusFailed,
				Output: output,
				Error:  err.Error(),
			}, nil
		}

		return &CommandResult{
			Status: CommandStatusSuccess,
			Output: output,
		}, nil
	}
}

// UninstallPackageExecutor creates an executor for package uninstallation
func UninstallPackageExecutor(uninstallFunc func(ctx context.Context, payload *UninstallPackagePayload) (string, error)) CommandExecutor {
	return func(ctx context.Context, payload interface{}) (*CommandResult, error) {
		var p UninstallPackagePayload
		if err := parseCommandPayload(payload, &p); err != nil {
			return nil, err
		}

		output, err := uninstallFunc(ctx, &p)
		if err != nil {
			return &CommandResult{
				Status: CommandStatusFailed,
				Output: output,
				Error:  err.Error(),
			}, nil
		}

		return &CommandResult{
			Status: CommandStatusSuccess,
			Output: output,
		}, nil
	}
}

// UpdatePackageExecutor creates an executor for package updates
func UpdatePackageExecutor(updateFunc func(ctx context.Context, payload *UpdatePackagePayload) (string, error)) CommandExecutor {
	return func(ctx context.Context, payload interface{}) (*CommandResult, error) {
		var p UpdatePackagePayload
		if err := parseCommandPayload(payload, &p); err != nil {
			return nil, err
		}

		output, err := updateFunc(ctx, &p)
		if err != nil {
			return &CommandResult{
				Status: CommandStatusFailed,
				Output: output,
				Error:  err.Error(),
			}, nil
		}

		return &CommandResult{
			Status: CommandStatusSuccess,
			Output: output,
		}, nil
	}
}

// QueryMetricsExecutor creates an executor for metrics queries
func QueryMetricsExecutor(queryFunc func(ctx context.Context, payload *QueryMetricsPayload) (string, error)) CommandExecutor {
	return func(ctx context.Context, payload interface{}) (*CommandResult, error) {
		var p QueryMetricsPayload
		if err := parseCommandPayload(payload, &p); err != nil {
			return nil, err
		}

		output, err := queryFunc(ctx, &p)
		if err != nil {
			return &CommandResult{
				Status: CommandStatusFailed,
				Output: output,
				Error:  err.Error(),
			}, nil
		}

		return &CommandResult{
			Status: CommandStatusSuccess,
			Output: output,
		}, nil
	}
}

// UpdateConfigExecutor creates an executor for config updates
func UpdateConfigExecutor(updateFunc func(ctx context.Context, payload *UpdateConfigPayload) (string, error)) CommandExecutor {
	return func(ctx context.Context, payload interface{}) (*CommandResult, error) {
		var p UpdateConfigPayload
		if err := parseCommandPayload(payload, &p); err != nil {
			return nil, err
		}

		output, err := updateFunc(ctx, &p)
		if err != nil {
			return &CommandResult{
				Status: CommandStatusFailed,
				Output: output,
				Error:  err.Error(),
			}, nil
		}

		return &CommandResult{
			Status: CommandStatusSuccess,
			Output: output,
		}, nil
	}
}

// ExecuteScriptExecutor creates an executor for script execution
func ExecuteScriptExecutor(execFunc func(ctx context.Context, payload *ExecuteScriptPayload) (string, error)) CommandExecutor {
	return func(ctx context.Context, payload interface{}) (*CommandResult, error) {
		var p ExecuteScriptPayload
		if err := parseCommandPayload(payload, &p); err != nil {
			return nil, err
		}

		output, err := execFunc(ctx, &p)
		if err != nil {
			return &CommandResult{
				Status: CommandStatusFailed,
				Output: output,
				Error:  err.Error(),
			}, nil
		}

		return &CommandResult{
			Status: CommandStatusSuccess,
			Output: output,
		}, nil
	}
}

// RunPeonExecutor creates an executor for running peon scripts from GitHub repositories
func RunPeonExecutor(runFunc func(ctx context.Context, payload *RunPeonPayload) (string, error)) CommandExecutor {
	return func(ctx context.Context, payload interface{}) (*CommandResult, error) {
		var p RunPeonPayload
		if err := parseCommandPayload(payload, &p); err != nil {
			return nil, err
		}

		output, err := runFunc(ctx, &p)
		if err != nil {
			return &CommandResult{
				Status: CommandStatusFailed,
				Output: output,
				Error:  err.Error(),
			}, nil
		}

		return &CommandResult{
			Status: CommandStatusSuccess,
			Output: output,
		}, nil
	}
}
