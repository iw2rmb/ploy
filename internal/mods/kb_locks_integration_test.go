//go:build integration
// +build integration

package mods

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// These tests require a running NATS JetStream server
// Skip if NATS_ADDR is not set or server is not available

func skipIfNoJetStream(t *testing.T) {
	natsAddr := os.Getenv("NATS_ADDR")
	if natsAddr == "" {
		t.Skip("NATS_ADDR not set, skipping JetStream integration tests")
	}

	// Try to connect to verify server is available
	conn, err := nats.Connect(natsAddr)
	if err != nil {
		t.Skipf("Cannot connect to NATS server at %s: %v", natsAddr, err)
	}
	defer conn.Close()

	// Verify JetStream is enabled
	_, err = conn.JetStream()
	if err != nil {
		t.Skipf("JetStream not enabled on NATS server: %v", err)
	}
}

func TestJetStreamKBLockManager_Integration_BasicLocking(t *testing.T) {
	skipIfNoJetStream(t)

	mgr, err := NewJetstreamKBLockManager()
	if err != nil {
		t.Fatalf("Failed to create JetStream lock manager: %v", err)
	}
	defer mgr.Close()

	ctx := context.Background()

	// Test lock acquisition
	lock, err := mgr.AcquireLock(ctx, "integration-test-1", 10*time.Second)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	if lock.Backend != "jetstream" {
		t.Errorf("Expected backend 'jetstream', got: %s", lock.Backend)
	}

	// Verify lock is held
	locked, err := mgr.IsLocked(ctx, "integration-test-1")
	if err != nil {
		t.Fatalf("Failed to check lock status: %v", err)
	}
	if !locked {
		t.Error("Expected key to be locked")
	}

	// Release lock
	err = mgr.ReleaseLock(ctx, lock)
	if err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}

	// Verify lock is released
	locked, err = mgr.IsLocked(ctx, "integration-test-1")
	if err != nil {
		t.Fatalf("Failed to check lock status after release: %v", err)
	}
	if locked {
		t.Error("Expected key to be unlocked after release")
	}
}

func TestJetStreamKBLockManager_Integration_Contention(t *testing.T) {
	skipIfNoJetStream(t)

	mgr1, err := NewJetstreamKBLockManager()
	if err != nil {
		t.Fatalf("Failed to create first JetStream lock manager: %v", err)
	}
	defer mgr1.Close()

	mgr2, err := NewJetstreamKBLockManager()
	if err != nil {
		t.Fatalf("Failed to create second JetStream lock manager: %v", err)
	}
	defer mgr2.Close()

	ctx := context.Background()
	lockKey := "integration-test-contention"

	// First manager acquires lock
	lock1, err := mgr1.AcquireLock(ctx, lockKey, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to acquire lock with first manager: %v", err)
	}

	// Second manager should fail to acquire same lock
	_, err = mgr2.AcquireLock(ctx, lockKey, 5*time.Second)
	if err == nil {
		t.Fatal("Expected second manager to fail acquiring already held lock")
	}

	// Release lock with first manager
	err = mgr1.ReleaseLock(ctx, lock1)
	if err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}

	// Second manager should now be able to acquire lock
	lock2, err := mgr2.AcquireLock(ctx, lockKey, 5*time.Second)
	if err != nil {
		t.Fatalf("Expected second manager to acquire lock after release: %v", err)
	}

	// Clean up
	err = mgr2.ReleaseLock(ctx, lock2)
	if err != nil {
		t.Fatalf("Failed to release second lock: %v", err)
	}
}

func TestJetStreamKBLockManager_Integration_ConcurrentAccess(t *testing.T) {
	skipIfNoJetStream(t)

	const numGoroutines = 10
	const lockKey = "integration-test-concurrent"

	var wg sync.WaitGroup
	successCount := make(chan int, numGoroutines)
	ctx := context.Background()

	// Launch multiple goroutines trying to acquire the same lock
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			mgr, err := NewJetstreamKBLockManager()
			if err != nil {
				t.Errorf("Goroutine %d: Failed to create lock manager: %v", id, err)
				return
			}
			defer mgr.Close()

			config := &LockConfig{
				DefaultTTL:    2 * time.Second,
				MaxWaitTime:   15 * time.Second,
				RetryInterval: 75 * time.Millisecond,
				MaxRetries:    60,
			}

			executed := false
			err = mgr.TryWithLockRetry(ctx, lockKey, config, func() error {
				executed = true
				// Hold the lock briefly
				time.Sleep(100 * time.Millisecond)
				return nil
			})

			if err == nil && executed {
				successCount <- 1
			}
		}(i)
	}

	wg.Wait()
	close(successCount)

	// Count successful executions
	totalSuccess := 0
	for success := range successCount {
		totalSuccess += success
	}

	// All goroutines should eventually succeed due to retries
	if totalSuccess != numGoroutines {
		t.Errorf("Expected all %d goroutines to succeed, got %d", numGoroutines, totalSuccess)
	}
}

func TestJetStreamKBLockManager_Integration_LockExpiry(t *testing.T) {
	skipIfNoJetStream(t)

	mgr, err := NewJetstreamKBLockManager()
	if err != nil {
		t.Fatalf("Failed to create JetStream lock manager: %v", err)
	}
	defer mgr.Close()

	ctx := context.Background()
	lockKey := "integration-test-expiry"

	// Acquire lock with short TTL
	lock, err := mgr.AcquireLock(ctx, lockKey, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Verify lock is held
	locked, err := mgr.IsLocked(ctx, lockKey)
	if err != nil {
		t.Fatalf("Failed to check lock status: %v", err)
	}
	if !locked {
		t.Error("Expected key to be locked")
	}

	// Wait for lock to expire
	time.Sleep(2 * time.Second)

	// Lock should be considered expired and cleaned up
	locked, err = mgr.IsLocked(ctx, lockKey)
	if err != nil {
		t.Fatalf("Failed to check lock status after expiry: %v", err)
	}
	if locked {
		t.Error("Expected key to be unlocked after TTL expiry")
	}

	// Should be able to acquire the same key again
	lock2, err := mgr.AcquireLock(ctx, lockKey, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to acquire lock after expiry: %v", err)
	}

	// Clean up
	err = mgr.ReleaseLock(ctx, lock2)
	if err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}

	// Original lock should fail to release (expired)
	err = mgr.ReleaseLock(ctx, lock)
	// This should succeed (idempotent) or fail gracefully
	if err != nil {
		t.Logf("Release of expired lock returned error (expected): %v", err)
	}
}

func TestJetStreamKBLockManager_Integration_EventPublishing(t *testing.T) {
	skipIfNoJetStream(t)

	mgr, err := NewJetstreamKBLockManager()
	if err != nil {
		t.Fatalf("Failed to create JetStream lock manager: %v", err)
	}
	defer mgr.Close()

	ctx := context.Background()
	lockKey := "integration-test-events"

	// Subscribe to lock events
	eventReceived := make(chan bool, 2)
	sub, err := mgr.conn.Subscribe("mods.kb.lock.*.*", func(msg *nats.Msg) {
		t.Logf("Received event on subject: %s", msg.Subject)
		eventReceived <- true
	})
	if err != nil {
		t.Fatalf("Failed to subscribe to events: %v", err)
	}
	defer sub.Unsubscribe()

	// Acquire lock (should trigger event)
	lock, err := mgr.AcquireLock(ctx, lockKey, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Release lock (should trigger event)
	err = mgr.ReleaseLock(ctx, lock)
	if err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}

	// Wait for events with timeout
	timeout := time.After(2 * time.Second)
	eventsReceived := 0

	for eventsReceived < 2 {
		select {
		case <-eventReceived:
			eventsReceived++
		case <-timeout:
			t.Fatalf("Expected 2 events (acquire + release), got %d", eventsReceived)
		}
	}

	t.Logf("Successfully received %d lock events", eventsReceived)
}

func TestJetStreamKBLockManager_Integration_MaintenanceEvents(t *testing.T) {
	skipIfNoJetStream(t)

	// This test verifies that maintenance scheduler can receive lock events
	// Create a lock manager
	mgr, err := NewJetstreamKBLockManager()
	if err != nil {
		t.Fatalf("Failed to create JetStream lock manager: %v", err)
	}
	defer mgr.Close()

	ctx := context.Background()

	// Subscribe to lock release events (simulating maintenance scheduler)
	eventReceived := make(chan string, 1)
	sub, err := mgr.conn.Subscribe("mods.kb.lock.released.*", func(msg *nats.Msg) {
		// Extract key from subject
		parts := strings.Split(msg.Subject, ".")
		if len(parts) >= 5 {
			eventReceived <- parts[4] // The kb identifier part
		}
	})
	if err != nil {
		t.Fatalf("Failed to subscribe to release events: %v", err)
	}
	defer sub.Unsubscribe()

	// Acquire and release a signature lock
	signatureKey := BuildSignatureLockKey("java", "test-signature")
	lock, err := mgr.AcquireLock(ctx, signatureKey, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to acquire signature lock: %v", err)
	}

	err = mgr.ReleaseLock(ctx, lock)
	if err != nil {
		t.Fatalf("Failed to release signature lock: %v", err)
	}

	// Wait for release event
	timeout := time.After(2 * time.Second)
	select {
	case receivedKey := <-eventReceived:
		if receivedKey != signatureKey {
			t.Errorf("Expected event for key %q, got %q", signatureKey, receivedKey)
		}
		t.Logf("Successfully received maintenance event for key: %s", receivedKey)
	case <-timeout:
		t.Fatal("Expected to receive lock release event for maintenance triggering")
	}
}
