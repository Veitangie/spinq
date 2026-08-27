// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0
package spinq

import (
	"errors"
	"testing"
)

func TestPanicError_ErrorRendersRecoveredValue(t *testing.T) {
	p := PanicError{underlying: "boom"}
	if got := p.Error(); got != "boom" {
		t.Errorf("expected Error() to render the recovered value, got %q", got)
	}
}

func TestPanicError_ValueReturnsOriginalRecoveredValue(t *testing.T) {
	original := errors.New("boom")
	p := PanicError{underlying: original}

	if got := p.Value(); got != any(original) {
		t.Errorf("expected Value() to return the original recovered value unwrapped, got %v", got)
	}
	if got := p.Error(); got != original.Error() {
		t.Errorf("expected Error() to render the recovered error's message, got %q", got)
	}
}

func TestSafeGetFrame_RecoversPanicAndReportsItOnErrCh(t *testing.T) {
	errCh := make(chan error, 1)
	st := &spinnerState{errCh: errCh}

	res, err := st.safeGetFrame(func() ([]byte, error) { panic("boom: deliberate panic") })

	if !errors.Is(err, ErrNoFrame) {
		t.Errorf("expected safeGetFrame to translate a panic into ErrNoFrame, got %v", err)
	}
	if len(res) != 0 {
		t.Errorf("expected an empty frame on panic, got %q", res)
	}

	select {
	case gotErr := <-errCh:
		var p PanicError
		if !errors.As(gotErr, &p) {
			t.Fatalf("expected a PanicError on errCh, got %v (%T)", gotErr, gotErr)
		}
		if p.Value() != "boom: deliberate panic" {
			t.Errorf("expected PanicError.Value() to carry the original recovered value, got %v", p.Value())
		}
	default:
		t.Fatal("expected the panic to already be buffered on errCh")
	}
}

func TestSafeGetFrame_PassesThroughNormalResult(t *testing.T) {
	st := &spinnerState{errCh: make(chan error, 1)}

	res, err := st.safeGetFrame(func() ([]byte, error) { return []byte("ok"), nil })

	if err != nil {
		t.Errorf("expected no error for a non-panicking getFrame, got %v", err)
	}
	if string(res) != "ok" {
		t.Errorf("expected the underlying result to pass through unchanged, got %q", res)
	}
}

func TestSafeGetFrameFunc_WrapsPanicsOfTheReturnedFunc(t *testing.T) {
	st := &spinnerState{errCh: make(chan error, 1)}
	wrapped := st.safeGetFrameFunc(func() ([]byte, error) { panic("boom") })

	res, err := wrapped()

	if !errors.Is(err, ErrNoFrame) {
		t.Errorf("expected the wrapped func to translate a panic into ErrNoFrame, got %v", err)
	}
	if len(res) != 0 {
		t.Errorf("expected an empty frame on panic, got %q", res)
	}
}
