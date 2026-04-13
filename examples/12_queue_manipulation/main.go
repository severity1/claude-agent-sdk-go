// Example demonstrating queue manipulation features
// Shows how to enqueue messages and remove them before they're sent (like "up arrow" in CLI)
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	claudecode "github.com/severity1/claude-agent-sdk-go"
)

func main() {
	fmt.Println("=== Queue Manipulation Example ===")

	// Create and connect client
	client := claudecode.NewClient()

	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Disconnect() }()

	// Create queue manager
	qm := claudecode.NewQueueManager(client)
	defer func() { _ = qm.Close() }()

	sessionID := "demo-session"

	fmt.Println("1. Enqueueing 3 messages (they will NOT be sent immediately)...")

	// Enqueue messages - they stay in queue
	msg1, err := qm.Enqueue(ctx, sessionID, "What is 2+2?")
	if err != nil {
		log.Fatal(err) //nolint:gocritic // intentional exit-on-error for example code
	}
	fmt.Printf("   ✓ Enqueued message 1: %s\n", msg1.ID)

	msg2, err := qm.Enqueue(ctx, sessionID, "What is 3+3?")
	if err != nil {
		log.Fatal(err) //nolint:gocritic // intentional exit-on-error for example code
	}
	fmt.Printf("   ✓ Enqueued message 2: %s\n", msg2.ID)

	msg3, err := qm.Enqueue(ctx, sessionID, "What is 4+4?")
	if err != nil {
		log.Fatal(err) //nolint:gocritic // intentional exit-on-error for example code
	}
	fmt.Printf("   ✓ Enqueued message 3: %s\n", msg3.ID)

	// Check queue length
	queueLength := qm.GetQueueLength(sessionID)
	fmt.Printf("\n2. Queue length: %d messages pending\n", queueLength)

	// Get queue status
	status, err := qm.GetQueueStatus(sessionID)
	if err != nil {
		log.Fatal(err) //nolint:gocritic // intentional exit-on-error for example code
	}
	fmt.Printf("   Status: %d pending\n", len(status.PendingMessages))

	// Remove message 3 (like pressing "up arrow" in CLI - removes most recent)
	fmt.Printf("\n3. Removing message 3 from queue (simulating 'up arrow' in CLI)...\n")
	fmt.Printf("   Note: 'Up arrow' removes the MOST RECENT message (last enqueued)\n")
	if err := qm.RemoveFromQueue(sessionID, msg3.ID); err != nil {
		log.Printf("   ✗ Failed to remove: %v", err)
	} else {
		fmt.Printf("   ✓ Message 3 removed successfully\n")
	}

	// Check queue length again
	queueLength = qm.GetQueueLength(sessionID)
	fmt.Printf("\n4. Queue length after removal: %d messages pending\n", queueLength)

	// Show pending messages
	status, _ = qm.GetQueueStatus(sessionID)
	fmt.Printf("   Pending messages:\n")
	for i, msg := range status.PendingMessages {
		fmt.Printf("     %d. %s (ID: %s)\n", i+1, msg.Content, msg.ID)
	}

	fmt.Printf("\n5. Messages will now be processed in order (only 1 and 2):\n")
	fmt.Printf("   Note: Processing happens in background. Message 3 was removed before sending.\n")

	// Wait a bit to see processing
	time.Sleep(2 * time.Second)

	// Show final status
	status, _ = qm.GetQueueStatus(sessionID)
	fmt.Printf("\n6. Final queue state:\n")
	fmt.Printf("   Pending: %d\n", len(status.PendingMessages))

	fmt.Println("\n✅ Queue manipulation complete!")
	fmt.Println("   Message 3 (most recent) was successfully removed from the queue before being sent.")
	fmt.Println("   This demonstrates the 'up arrow' functionality from Claude CLI.")
	fmt.Println("   Up arrow removes the LAST message you added to the queue.")
}
