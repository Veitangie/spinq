// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0
package spinq

import (
	"errors"
	"testing"
	"time"
)

func TestFireEvent_GivesUpAfterTimeoutWhenNoReaderIsReady(t *testing.T) {
	ch := make(chan error)

	start := time.Now()
	fireEvent[error](errors.New("boom"), ch)
	elapsed := time.Since(start)

	if elapsed < 8*time.Millisecond {
		t.Errorf("expected fireEvent to wait out its ~10ms timeout before giving up, returned after %v", elapsed)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("expected fireEvent to give up promptly once its timeout elapses, took %v", elapsed)
	}
}

func TestFireEvent_DeliversImmediatelyToAReadyReader(t *testing.T) {
	ch := make(chan error, 1)

	start := time.Now()
	fireEvent[error](errors.New("boom"), ch)
	elapsed := time.Since(start)

	if elapsed > 5*time.Millisecond {
		t.Errorf("expected fireEvent to deliver immediately to a ready receiver, took %v", elapsed)
	}

	select {
	case err := <-ch:
		if err.Error() != "boom" {
			t.Errorf("expected the delivered value to pass through unchanged, got %v", err)
		}
	default:
		t.Fatal("expected the event to have been delivered")
	}
}
