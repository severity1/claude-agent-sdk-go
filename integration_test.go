//go:build integration

package claudecode_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/severity1/claude-agent-sdk-go"
)

// TestIntegrationCoreQueries tests core query functionality end-to-end.
func TestIntegrationCoreQueries(t *testing.T) {
	tests := []struct {
		name       string
		scenario   string
		setupTest  func(*testing.T) (*integrationMockTransport, *integrationTestOptions)
		validateFn func(*testing.T, context.Context, claudecode.MessageIterator, *integrationMockTransport)
	}{
		{
			name:     "simple_query_response",
			scenario: "simple_query",
			setupTest: func(t *testing.T) (*integrationMockTransport, *integrationTestOptions) {
				transport := newIntegrationMockTransport(
					WithIntegrationSimpleResponse("The answer is 42."),
				)
				opts := &integrationTestOptions{timeout: 3 * time.Second}
				return transport, opts
			},
			validateFn: validateSimpleQueryResponse,
		},
		{
			name:     "query_with_tools",
			scenario: "tool_usage",
			setupTest: func(t *testing.T) (*integrationMockTransport, *integrationTestOptions) {
				transport := newIntegrationMockTransport(
					WithIntegrationToolUsage(),
				)
				opts := &integrationTestOptions{timeout: 5 * time.Second}
				return transport, opts
			},
			validateFn: validateQueryWithTools,
		},
		{
			name:     "streaming_client_integration",
			scenario: "streaming",
			setupTest: func(t *testing.T) (*integrationMockTransport, *integrationTestOptions) {
				transport := newIntegrationMockTransport(
					WithIntegrationStreamingResponse("Streaming response part 1", "Streaming response part 2"),
				)
				opts := &integrationTestOptions{timeout: 20 * time.Second, streaming: true}
				return transport, opts
			},
			validateFn: validateStreamingClient,
		},
		{
			name:     "interrupt_during_streaming",
			scenario: "interrupt",
			setupTest: func(t *testing.T) (*integrationMockTransport, *integrationTestOptions) {
				transport := newIntegrationMockTransport(
					WithIntegrationInterruptScenario(),
				)
				opts := &integrationTestOptions{timeout: 10 * time.Second, streaming: true}
				return transport, opts
			},
			validateFn: validateInterruptDuringStreaming,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport, opts := test.setupTest(t)

			ctx, cancel := setupIntegrationContext(t, opts.timeout)
			defer cancel()

			if opts.streaming {
				// Test streaming client functionality
				client := claudecode.NewClientWithTransport(transport)
				defer func() {
					disconnectIntegrationClientSafely(t, client)
					// Verify resource cleanup after client is disconnected
					verifyIntegrationResourceCleanup(t, transport)
				}()

				connectIntegrationClientSafely(t, ctx, client)
				iter := client.ReceiveResponse(ctx)
				defer iter.Close()

				test.validateFn(t, ctx, iter, transport)
			} else {
				// Test query functionality
				iter, err := claudecode.QueryWithTransport(ctx, "test query", transport)
				if err != nil {
					t.Fatalf("Query failed: %v", err)
				}
				defer func() {
					iter.Close()
					// Verify resource cleanup after iterator is closed
					verifyIntegrationResourceCleanup(t, transport)
				}()

				test.validateFn(t, ctx, iter, transport)
			}
		})
	}
}

// TestIntegrationSessionManagement tests session and configuration management
// Covers T168: Session Continuation, T169: MCP Integration, T170: Permission Mode, T171: Working Directory
func TestIntegrationSessionManagement(t *testing.T) {
	tests := []struct {
		name       string
		setupTest  func(*testing.T) (*integrationMockTransport, []claudecode.Option)
		validateFn func(*testing.T, context.Context, claudecode.Client, *integrationMockTransport)
	}{
		{
			name: "session_continuation",
			setupTest: func(t *testing.T) (*integrationMockTransport, []claudecode.Option) {
				transport := newIntegrationMockTransport(
					WithIntegrationSessionContinuation("session-continuation-123"),
				)
				opts := []claudecode.Option{
					claudecode.WithContinueConversation(true),
					claudecode.WithResume("session-continuation-123"),
				}
				return transport, opts
			},
			validateFn: validateSessionContinuation,
		},
		{
			name: "mcp_integration",
			setupTest: func(t *testing.T) (*integrationMockTransport, []claudecode.Option) {
				transport := newIntegrationMockTransport(
					WithIntegrationMCPServers(),
				)
				mcpServers := map[string]claudecode.McpServerConfig{
					"test_server": &claudecode.McpStdioServerConfig{
						Type:    claudecode.McpServerTypeStdio,
						Command: "python",
						Args:    []string{"-m", "test_mcp_server"},
					},
				}
				opts := []claudecode.Option{
					claudecode.WithMcpServers(mcpServers),
				}
				return transport, opts
			},
			validateFn: validateMCPIntegration,
		},
		{
			name: "permission_mode_integration",
			setupTest: func(t *testing.T) (*integrationMockTransport, []claudecode.Option) {
				transport := newIntegrationMockTransport(
					WithIntegrationPermissionModes(),
				)
				opts := []claudecode.Option{
					claudecode.WithPermissionMode(claudecode.PermissionModeAcceptEdits),
				}
				return transport, opts
			},
			validateFn: validatePermissionMode,
		},
		{
			name: "working_directory_integration",
			setupTest: func(t *testing.T) (*integrationMockTransport, []claudecode.Option) {
				// Use existing /tmp directory instead of non-existent /tmp/test
				transport := newIntegrationMockTransport(
					WithIntegrationWorkingDirectory("/tmp"),
				)
				opts := []claudecode.Option{
					claudecode.WithCwd("/tmp"),
					claudecode.WithAddDirs("/tmp"),
				}
				return transport, opts
			},
			validateFn: validateWorkingDirectory,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport, options := test.setupTest(t)

			ctx, cancel := setupIntegrationContext(t, 5*time.Second)
			defer cancel()

			client := claudecode.NewClientWithTransport(transport, options...)
			defer func() {
				disconnectIntegrationClientSafely(t, client)
				// Verify resource cleanup after client is disconnected
				verifyIntegrationResourceCleanup(t, transport)
			}()

			connectIntegrationClientSafely(t, ctx, client)

			test.validateFn(t, ctx, client, transport)
		})
	}
}

// TestIntegrationReliability tests error handling and resource management
// Covers T172: Error Handling, T173: Large Response, T174: Concurrent Clients, T175: Resource Cleanup
func TestIntegrationReliability(t *testing.T) {
	tests := []struct {
		name       string
		setupTest  func(*testing.T) (*integrationMockTransport, *integrationTestOptions)
		validateFn func(*testing.T, context.Context, *integrationMockTransport, *integrationTestOptions)
	}{
		{
			name: "error_handling_integration",
			setupTest: func(t *testing.T) (*integrationMockTransport, *integrationTestOptions) {
				transport := newIntegrationMockTransport(
					WithIntegrationErrorScenarios(),
				)
				opts := &integrationTestOptions{timeout: 3 * time.Second}
				return transport, opts
			},
			validateFn: validateErrorHandling,
		},
		{
			name: "large_response_integration",
			setupTest: func(t *testing.T) (*integrationMockTransport, *integrationTestOptions) {
				transport := newIntegrationMockTransport(
					WithIntegrationLargeResponse(1024 * 1024), // 1MB response
				)
				opts := &integrationTestOptions{timeout: 8 * time.Second}
				return transport, opts
			},
			validateFn: validateLargeResponse,
		},
		{
			name: "concurrent_clients_integration",
			setupTest: func(t *testing.T) (*integrationMockTransport, *integrationTestOptions) {
				transport := newIntegrationMockTransport(
					WithIntegrationConcurrentScenario(10), // 10 concurrent clients
				)
				opts := &integrationTestOptions{timeout: 30 * time.Second, concurrent: 10}
				return transport, opts
			},
			validateFn: validateConcurrentClients,
		},
		{
			name: "resource_cleanup_integration",
			setupTest: func(t *testing.T) (*integrationMockTransport, *integrationTestOptions) {
				transport := newIntegrationMockTransport(
					WithIntegrationResourceTracking(),
				)
				opts := &integrationTestOptions{timeout: 5 * time.Second}
				return transport, opts
			},
			validateFn: validateResourceCleanup,
		},
		{
			name: "strict_resource_cleanup_test",
			setupTest: func(t *testing.T) (*integrationMockTransport, *integrationTestOptions) {
				transport := newIntegrationMockTransport(
					WithIntegrationResourceTracking(),
				)
				opts := &integrationTestOptions{timeout: 5 * time.Second}
				return transport, opts
			},
			validateFn: validateSpecificResourceCleanup,
		},
		{
			name: "strict_transport_state_test",
			setupTest: func(t *testing.T) (*integrationMockTransport, *integrationTestOptions) {
				transport := newIntegrationMockTransport()
				opts := &integrationTestOptions{timeout: 3 * time.Second}
				return transport, opts
			},
			validateFn: validateStrictTransportState,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport, opts := test.setupTest(t)

			ctx, cancel := setupIntegrationContext(t, opts.timeout)
			defer cancel()

			// Verify resource cleanup after validation function's defers complete
			defer verifyIntegrationResourceCleanup(t, transport)

			test.validateFn(t, ctx, transport, opts)
		})
	}
}

// TestIntegrationPerformance tests performance and load scenarios
// Covers T176: Performance Integration, T177: Stress Test Integration
func TestIntegrationPerformance(t *testing.T) {
	tests := []struct {
		name       string
		setupTest  func(*testing.T) (*integrationMockTransport, *integrationTestOptions)
		validateFn func(*testing.T, context.Context, *integrationMockTransport, *integrationTestOptions)
	}{
		{
			name: "performance_integration_test",
			setupTest: func(t *testing.T) (*integrationMockTransport, *integrationTestOptions) {
				transport := newIntegrationMockTransport(
					WithIntegrationPerformanceScenario(1000), // 1000 messages
				)
				opts := &integrationTestOptions{
					timeout:      60 * time.Second,
					messageCount: 1000,
					maxLatencyMs: 100,
					maxMemoryMB:  50,
				}
				return transport, opts
			},
			validateFn: validatePerformance,
		},
		{
			name: "stress_test_integration",
			setupTest: func(t *testing.T) (*integrationMockTransport, *integrationTestOptions) {
				transport := newIntegrationMockTransport(
					WithIntegrationStressScenario(5000, 20), // 5000 messages, 20 concurrent
				)
				opts := &integrationTestOptions{
					timeout:      120 * time.Second,
					messageCount: 5000,
					concurrent:   20,
					maxLatencyMs: 500,
					maxMemoryMB:  100,
				}
				return transport, opts
			},
			validateFn: validateStressTest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport, opts := test.setupTest(t)

			ctx, cancel := setupIntegrationContext(t, opts.timeout)
			defer cancel()

			// Verify resource cleanup after validation function's defers complete
			defer verifyIntegrationResourceCleanup(t, transport)

			test.validateFn(t, ctx, transport, opts)
		})
	}
}

// TestIntegrationPlatforms tests platform compatibility
// Covers T178: CLI Version Compatibility, T179: Cross-Platform Integration
func TestIntegrationPlatforms(t *testing.T) {
	tests := []struct {
		name       string
		setupTest  func(*testing.T) (*integrationMockTransport, *integrationTestOptions)
		validateFn func(*testing.T, context.Context, *integrationMockTransport, *integrationTestOptions)
	}{
		{
			name: "cli_version_compatibility",
			setupTest: func(t *testing.T) (*integrationMockTransport, *integrationTestOptions) {
				transport := newIntegrationMockTransport(
					WithIntegrationCLIVersions("1.0.0", "1.1.0", "1.2.0"),
				)
				opts := &integrationTestOptions{timeout: 5 * time.Second}
				return transport, opts
			},
			validateFn: validateCLIVersionCompatibility,
		},
		{
			name: "cross_platform_integration",
			setupTest: func(t *testing.T) (*integrationMockTransport, *integrationTestOptions) {
				transport := newIntegrationMockTransport(
					WithIntegrationPlatformScenarios(),
				)
				opts := &integrationTestOptions{timeout: 5 * time.Second}
				return transport, opts
			},
			validateFn: validateCrossPlatform,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport, opts := test.setupTest(t)

			ctx, cancel := setupIntegrationContext(t, opts.timeout)
			defer cancel()

			test.validateFn(t, ctx, transport, opts)

			verifyIntegrationResourceCleanup(t, transport)
		})
	}
}

// TestIntegrationProduction tests production scenarios and edge cases
// Covers T180: Network Isolation, T181: Full Feature Integration
func TestIntegrationProduction(t *testing.T) {
	tests := []struct {
		name       string
		setupTest  func(*testing.T) (*integrationMockTransport, *integrationTestOptions)
		validateFn func(*testing.T, context.Context, *integrationMockTransport, *integrationTestOptions)
	}{
		{
			name: "network_isolation_integration",
			setupTest: func(t *testing.T) (*integrationMockTransport, *integrationTestOptions) {
				transport := newIntegrationMockTransport(
					WithIntegrationNetworkIsolation(),
				)
				opts := &integrationTestOptions{timeout: 5 * time.Second}
				return transport, opts
			},
			validateFn: validateNetworkIsolation,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport, opts := test.setupTest(t)

			ctx, cancel := setupIntegrationContext(t, opts.timeout)
			defer cancel()

			test.validateFn(t, ctx, transport, opts)

			verifyIntegrationResourceCleanup(t, transport)
		})
	}
}

// integrationTestOptions holds configuration for integration tests
type integrationTestOptions struct {
	timeout      time.Duration
	streaming    bool
	concurrent   int
	messageCount int
	maxLatencyMs int
	maxMemoryMB  int
	fullFeature  bool
}

// integrationMockTransport implements Transport interface for integration testing
// Following the established pattern from client_test.go
type integrationMockTransport struct {
	mu           sync.Mutex
	connected    bool
	closed       bool
	sentMessages []claudecode.StreamMessage

	// Integration-specific test data
	testMessages    []claudecode.Message
	testScenarios   map[string]*integrationScenario
	resourceTracker *integrationResourceTracker
	msgChan         chan claudecode.Message
	errChan         chan error
	messageIndex    int // Track which messages have been sent

	// Error injection for testing
	connectError   error
	sendError      error
	interruptError error
	closeError     error
}

// integrationScenario represents a test scenario configuration
type integrationScenario struct {
	name           string
	messages       []claudecode.Message
	expectedErrors []error
	sessionID      string
	mcpServers     bool
	workingDir     string
}

// integrationResourceTracker tracks resources for leak detection
type integrationResourceTracker struct {
	mu                sync.Mutex
	goroutines        int
	allocatedMemoryMB int
	openFiles         int
	connections       int
}

func (i *integrationMockTransport) Connect(ctx context.Context) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.connectError != nil {
		return i.connectError
	}
	i.connected = true
	return nil
}

func (i *integrationMockTransport) SendMessage(ctx context.Context, message claudecode.StreamMessage) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.sendError != nil {
		return i.sendError
	}
	if !i.connected {
		return fmt.Errorf("not connected")
	}
	i.sentMessages = append(i.sentMessages, message)

	return nil
}

func (i *integrationMockTransport) ReceiveMessages(ctx context.Context) (<-chan claudecode.Message, <-chan error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.closed {
		closedMsgChan := make(chan claudecode.Message)
		closedErrChan := make(chan error)
		close(closedMsgChan)
		close(closedErrChan)
		return closedMsgChan, closedErrChan
	}

	// Initialize channels ONCE if not already done (streaming pattern)
	if i.msgChan == nil {
		i.msgChan = make(chan claudecode.Message, 100)
		i.errChan = make(chan error, 10)

		// Pre-load all test messages immediately for simpler, more predictable behavior
		// This eliminates complex injection logic and timing issues
		if len(i.testMessages) > 0 {
			go func() {
				defer func() {
					// Safely close channel with proper locking
					i.mu.Lock()
					defer i.mu.Unlock()
					if i.msgChan != nil && !i.closed {
						close(i.msgChan)
					}
				}()
				for _, msg := range i.testMessages {
					i.mu.Lock()
					ch := i.msgChan
					closed := i.closed
					i.mu.Unlock()

					if closed || ch == nil {
						return
					}

					// Use recover to gracefully handle send on closed channel
					func() {
						defer func() {
							if r := recover(); r != nil {
								// Send on closed channel, exit gracefully
								return
							}
						}()
						select {
						case ch <- msg:
						case <-ctx.Done():
							return
						}
					}()
				}
			}()
		} else {
			// If no test messages, close immediately so iterator knows to stop
			close(i.msgChan)
		}

		// Send connection error if configured
		if i.connectError != nil {
			i.errChan <- i.connectError
		}

		// Track resources for the single channel creation
		if i.resourceTracker != nil {
			i.resourceTracker.mu.Lock()
			i.resourceTracker.connections = 1
			i.resourceTracker.mu.Unlock()
		}
	}

	// Always return the same persistent channels
	return i.msgChan, i.errChan
}

func (i *integrationMockTransport) Interrupt(ctx context.Context) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.interruptError != nil {
		return i.interruptError
	}
	return nil
}

func (i *integrationMockTransport) EndInput(_ context.Context) error {
	return nil
}

func (i *integrationMockTransport) Close() error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.closeError != nil {
		return i.closeError
	}

	if i.closed {
		return nil
	}

	i.connected = false
	i.closed = true

	// Close persistent channels if they exist
	if i.msgChan != nil {
		// Use recover to handle potential double-close panic gracefully
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Channel was already closed, which is fine
				}
			}()
			close(i.msgChan)
		}()
		i.msgChan = nil
	}
	if i.errChan != nil {
		close(i.errChan)
		i.errChan = nil
	}

	if i.resourceTracker != nil {
		i.resourceTracker.mu.Lock()
		// Clean up all tracked resources as expected
		i.resourceTracker.goroutines = 0        // All goroutines should be cleaned up
		i.resourceTracker.openFiles = 0         // All files closed
		i.resourceTracker.connections = 0       // All connections closed
		i.resourceTracker.allocatedMemoryMB = 0 // Memory released
		i.resourceTracker.mu.Unlock()
	}

	return nil
}
