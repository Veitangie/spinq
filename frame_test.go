// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0
package spinq

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSpinnerStatePresets(t *testing.T) {
	presets := map[string][]string{
		"DotsStates":      DotsStates,
		"LineStates":      LineStates,
		"ArrowStates":     ArrowStates,
		"PipeStates":      PipeStates,
		"FlyTroughStates": FlyTroughStates,
		"BounceStates":    BounceStates,
		"GrowingStates":   GrowingStates,
		"BinaryStates":    BinaryStates,
	}

	for name, states := range presets {
		t.Run(name, func(t *testing.T) {
			if len(states) == 0 {
				t.Fatalf("%s must not be empty", name)
			}

			f := Simple(states)
			first, err := f()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(first) == 0 {
				t.Fatalf("%s[0] must not be an empty frame", name)
			}

			for range len(states) - 1 {
				if _, err := f(); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			looped, err := f()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(looped) != string(first) {
				t.Errorf("expected %s to loop back to its first frame %q after %d calls, got %q", name, first, len(states), looped)
			}
		})
	}
}

func TestProgress_Success(t *testing.T) {
	f := Progress(func() (int, int) { return 5, 10 }, func(current, total int) []byte {
		return []byte("5/10")
	})

	got, err := f()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "5/10" {
		t.Errorf("expected %q, got %q", "5/10", got)
	}
}

func TestProgress_NonPositiveTotalReturnsErrNoFrame(t *testing.T) {
	called := false
	render := func(current, total int) []byte {
		called = true
		return []byte("x")
	}

	for _, total := range []int{0, -1, -100} {
		called = false
		f := Progress(func() (int, int) { return 0, total }, render)
		got, err := f()
		if !errors.Is(err, ErrNoFrame) {
			t.Errorf("total=%d: expected ErrNoFrame, got %v", total, err)
		}
		if len(got) != 0 {
			t.Errorf("total=%d: expected no bytes, got %q", total, got)
		}
		if called {
			t.Errorf("total=%d: render must not be called when total is non-positive", total)
		}
	}
}

func TestProgress_CurrentGreaterThanTotalReturnsErrNoFrame(t *testing.T) {
	called := false
	render := func(current, total int) []byte {
		called = true
		return []byte("x")
	}

	f := Progress(func() (int, int) { return 11, 10 }, render)
	got, err := f()
	if !errors.Is(err, ErrNoFrame) {
		t.Errorf("expected ErrNoFrame, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no bytes, got %q", got)
	}
	if called {
		t.Error("render must not be called when current > total")
	}
}

func TestProgress_IntegratesWithJoinPreservingLastGoodSegment(t *testing.T) {
	total := 10
	f := Join(" ", Progress(func() (int, int) { return 5, total }, func(current, total int) []byte {
		return []byte("bar-at-5")
	}), Static("label"))

	got, err := f()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "bar-at-5 label" {
		t.Fatalf("expected %q, got %q", "bar-at-5 label", got)
	}

	total = 0
	got, err = f()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(got), "bar-at-5") {
		t.Errorf("expected Join to keep the last good progress segment when total became invalid, got %q", got)
	}
}

func TestJoin_KeepsNonNilFuncsWhenSomeAreNil(t *testing.T) {
	f := Join(" ", Static("a"), nil, Static("b"))

	got, err := f()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "a b" {
		t.Errorf("expected %q, got %q", "a b", got)
	}
}

func TestDuration_NilTimerReturnsNoop(t *testing.T) {
	got, err := Duration(nil)()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected an empty frame, got %q", got)
	}
}

func TestDuration_DefaultFormatMeasuresFromFirstCall(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := base
	f := Duration(func() time.Time { return clock })

	got, err := f()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "0.0s" {
		t.Errorf("got %q, want %q", got, "0.0s")
	}

	clock = base.Add(2300 * time.Millisecond)
	got, err = f()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "2.3s" {
		t.Errorf("got %q, want %q", got, "2.3s")
	}

	clock = base.Add(10 * time.Second)
	got, err = f()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "10.0s" {
		t.Errorf("got %q, want %q", got, "10.0s")
	}
}

func TestDuration_CustomFormat(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := base
	f := Duration(func() time.Time { return clock }, DurationWithFormat(func(d time.Duration) string {
		return fmt.Sprintf("elapsed=%dms", d.Milliseconds())
	}))

	if _, err := f(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	clock = base.Add(150 * time.Millisecond)
	got, err := f()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "elapsed=150ms" {
		t.Errorf("got %q, want %q", got, "elapsed=150ms")
	}
}

func TestDuration_ConstructionToFirstCallGapDoesNotCount(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := base
	f := Duration(func() time.Time { return clock })

	clock = base.Add(5 * time.Minute)

	got, err := f()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "0.0s" {
		t.Errorf("expected the construction-to-first-call gap not to count, got %q, want %q", got, "0.0s")
	}

	clock = clock.Add(2300 * time.Millisecond)
	got, err = f()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "2.3s" {
		t.Errorf("got %q, want %q", got, "2.3s")
	}
}

func TestDuration_ExplicitStartAtSkipsConstructionTimeTimerCall(t *testing.T) {
	var timerCalls int
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	explicitStart := base.Add(-5 * time.Second)
	clock := base
	timer := func() time.Time {
		timerCalls++
		return clock
	}

	f := Duration(timer, DurationStartAt(explicitStart))
	if timerCalls != 0 {
		t.Errorf("expected DurationStartAt to skip the construction-time timer() call, but timer was called %d time(s)", timerCalls)
	}

	got, err := f()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "5.0s" {
		t.Errorf("got %q, want %q", got, "5.0s")
	}
}

func TestDuration_NowBeforeStartAtReturnsEmptyFrame(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	future := base.Add(1 * time.Hour)
	clock := base

	f := Duration(func() time.Time { return clock }, DurationStartAt(future))

	got, err := f()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected an empty frame when now is before startAt, got %q", got)
	}
}

func TestNoop(t *testing.T) {
	got, err := Noop()()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected an empty frame, got %q", got)
	}
}

func TestSimple_EmptyStatesReturnsNoop(t *testing.T) {
	got, err := Simple(nil)()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected an empty frame, got %q", got)
	}
}

func TestSimple_SingleStateReturnsStaticEveryCall(t *testing.T) {
	f := Simple([]string{"only"})
	for i := range 3 {
		got, err := f()
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if string(got) != "only" {
			t.Errorf("call %d: got %q, want %q", i, got, "only")
		}
	}
}

func TestSimpleOnceEvery_EmptyStatesReturnsNoop(t *testing.T) {
	got, err := SimpleOnceEvery(nil, 3)()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected an empty frame, got %q", got)
	}
}

func TestSimpleOnceEvery_NonPositiveModReturnsNoop(t *testing.T) {
	for _, mod := range []int{0, -1, -100} {
		got, err := SimpleOnceEvery([]string{"a", "b"}, mod)()
		if err != nil {
			t.Errorf("mod=%d: unexpected error: %v", mod, err)
		}
		if len(got) != 0 {
			t.Errorf("mod=%d: expected an empty frame, got %q", mod, got)
		}
	}
}

func TestSimpleOnceEvery_SingleStateReturnsStatic(t *testing.T) {
	f := SimpleOnceEvery([]string{"only"}, 3)
	for i := range 5 {
		got, err := f()
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if string(got) != "only" {
			t.Errorf("call %d: got %q, want %q", i, got, "only")
		}
	}
}

func TestSimpleOnceEvery_ModOneAdvancesEveryCall(t *testing.T) {
	f := SimpleOnceEvery([]string{"a", "b", "c"}, 1)
	want := []string{"a", "b", "c", "a", "b"}
	for i, w := range want {
		got, err := f()
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if string(got) != w {
			t.Errorf("call %d: got %q, want %q", i, got, w)
		}
	}
}

func TestSimpleOnceEvery_EachStateShownForExactlyModCalls(t *testing.T) {
	f := SimpleOnceEvery([]string{"a", "b", "c"}, 3)
	want := []string{"a", "a", "a", "b", "b", "b", "c", "c", "c"}
	for i, w := range want {
		got, err := f()
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if string(got) != w {
			t.Errorf("call %d: got %q, want %q (each state must be shown for exactly mod=3 calls before advancing)", i, got, w)
		}
	}
}

func TestRandom_EmptyStatesReturnsNoop(t *testing.T) {
	got, err := Random(nil)()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected an empty frame, got %q", got)
	}
}

func TestRandom_SingleStateReturnsStatic(t *testing.T) {
	f := Random([]string{"only"})
	for i := range 3 {
		got, err := f()
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if string(got) != "only" {
			t.Errorf("call %d: got %q, want %q", i, got, "only")
		}
	}
}

func TestRandom_UsesProvidedRandDeterministically(t *testing.T) {
	states := []string{"a", "b", "c", "d"}
	f := Random(states, rand.New(rand.NewPCG(1, 2)))
	mirror := rand.New(rand.NewPCG(1, 2))

	for i := range 20 {
		got, err := f()
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		want := states[mirror.IntN(len(states))]
		if string(got) != want {
			t.Errorf("call %d: got %q, want %q", i, got, want)
		}
	}
}

func TestRandom_IgnoresRandsBeyondFirst(t *testing.T) {
	states := []string{"a", "b", "c", "d"}
	primary := rand.New(rand.NewPCG(9, 9))
	decoy := rand.New(rand.NewPCG(1234, 5678))
	f := Random(states, primary, decoy)
	mirror := rand.New(rand.NewPCG(9, 9))

	for i := range 10 {
		got, err := f()
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		want := states[mirror.IntN(len(states))]
		if string(got) != want {
			t.Errorf("call %d: got %q, want %q — extra Rand args beyond the first must be ignored", i, got, want)
		}
	}
}

func TestRandom_FallsBackToPackageLevelRandWhenNoneProvided(t *testing.T) {
	states := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	f := Random(states)

	seen := map[string]bool{}
	for i := range 300 {
		got, err := f()
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if !slices.Contains(states, string(got)) {
			t.Fatalf("call %d: got %q, not one of the configured states", i, got)
		}
		seen[string(got)] = true
	}
	if len(seen) < 2 {
		t.Errorf("expected multiple distinct states across 300 calls with no Rand provided, saw only %v", seen)
	}
}

func TestRandomOnceEvery_EmptyStatesReturnsNoop(t *testing.T) {
	got, err := RandomOnceEvery(nil, 3)()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected an empty frame, got %q", got)
	}
}

func TestRandomOnceEvery_NonPositiveModReturnsNoop(t *testing.T) {
	for _, mod := range []int{0, -1, -100} {
		got, err := RandomOnceEvery([]string{"a", "b"}, mod)()
		if err != nil {
			t.Errorf("mod=%d: unexpected error: %v", mod, err)
		}
		if len(got) != 0 {
			t.Errorf("mod=%d: expected an empty frame, got %q", mod, got)
		}
	}
}

func TestRandomOnceEvery_SingleStateReturnsStatic(t *testing.T) {
	f := RandomOnceEvery([]string{"only"}, 3)
	for i := range 5 {
		got, err := f()
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if string(got) != "only" {
			t.Errorf("call %d: got %q, want %q", i, got, "only")
		}
	}
}

func TestRandomOnceEvery_FallsBackToPackageLevelRandWhenNoneProvided(t *testing.T) {
	const mod = 2
	states := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	f := RandomOnceEvery(states, mod)

	seen := map[string]bool{}
	for i := range 300 {
		got, err := f()
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if !slices.Contains(states, string(got)) {
			t.Fatalf("call %d: got %q, not one of the configured states", i, got)
		}
		seen[string(got)] = true
	}
	if len(seen) < 2 {
		t.Errorf("expected multiple distinct states across 300 calls with no Rand provided, saw only %v", seen)
	}
}

func TestRandomOnceEvery_ModOneDelegatesToRandomPerCall(t *testing.T) {
	states := []string{"a", "b", "c", "d"}
	fA := RandomOnceEvery(states, 1, rand.New(rand.NewPCG(11, 22)))
	fB := Random(states, rand.New(rand.NewPCG(11, 22)))

	for i := range 10 {
		gotA, err := fA()
		if err != nil {
			t.Fatalf("call %d: unexpected error from RandomOnceEvery: %v", i, err)
		}
		gotB, err := fB()
		if err != nil {
			t.Fatalf("call %d: unexpected error from Random: %v", i, err)
		}
		if string(gotA) != string(gotB) {
			t.Errorf("call %d: RandomOnceEvery(mod=1) gave %q, Random gave %q — mod=1 must behave exactly like Random", i, gotA, gotB)
		}
	}
}

func TestRandomOnceEvery_EachDrawShownForExactlyModCalls(t *testing.T) {
	const mod = 3
	states := []string{"a", "b", "c", "d"}
	f := RandomOnceEvery(states, mod, rand.New(rand.NewPCG(5, 5)))

	seq := make([]string, 12)
	for i := range seq {
		got, err := f()
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		seq[i] = string(got)
	}

	changed := false
	for i := 0; i < len(seq); i += mod {
		run := seq[i : i+mod]
		for _, v := range run {
			if v != run[0] {
				t.Errorf("run starting at call %d is not uniform: %v (each draw must be shown for exactly mod=%d calls)", i, run, mod)
			}
		}
		if i > 0 && run[0] != seq[i-mod] {
			changed = true
		}
	}
	if !changed {
		t.Errorf("expected the drawn state to change between runs, got constant sequence %v", seq)
	}
}

func TestRandomOnceEvery_SameSeedProducesSameSequence(t *testing.T) {
	states := []string{"a", "b", "c", "d", "e"}
	newFn := func() FrameFunc {
		return RandomOnceEvery(states, 4, rand.New(rand.NewPCG(123, 456)))
	}
	f1, f2 := newFn(), newFn()

	for i := range 20 {
		g1, err := f1()
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		g2, err := f2()
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if string(g1) != string(g2) {
			t.Fatalf("call %d: same-seeded RandomOnceEvery instances diverged: %q vs %q", i, g1, g2)
		}
	}
}

func TestSurrounded_PrependsPrefixAndAppendsSuffix(t *testing.T) {
	f := Surrounded("[", Static("mid"), "]")
	got, err := f()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "[mid]" {
		t.Errorf("got %q, want %q", got, "[mid]")
	}
}

func TestSurrounded_NilDelegateReturnsNoop(t *testing.T) {
	got, err := Surrounded("[", nil, "]")()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected an empty frame, got %q", got)
	}
}

func TestSurrounded_PropagatesDelegateError(t *testing.T) {
	wantErr := errors.New("boom")
	got, err := Surrounded("[", errorFrame(wantErr), "]")()
	if !errors.Is(err, wantErr) {
		t.Errorf("expected error %v, got %v", wantErr, err)
	}
	if len(got) != 0 {
		t.Errorf("expected no bytes on error, got %q", got)
	}
}
