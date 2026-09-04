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
		"DotsStates":       DotsStates,
		"LineStates":       LineStates,
		"ArrowStates":      ArrowStates,
		"PipeStates":       PipeStates,
		"FlyThroughStates": FlyThroughStates,
		"BounceStates":     BounceStates,
		"GrowingStates":    GrowingStates,
		"BinaryStates":     BinaryStates,
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

func TestProgress_NilFuncsReturnNoop(t *testing.T) {
	render := func(current, total int) []byte { return []byte("x") }
	progress := func() (int, int) { return 1, 10 }

	for name, f := range map[string]FrameFunc{
		"nil progress": Progress(nil, render),
		"nil render":   Progress(progress, nil),
		"both nil":     Progress(nil, nil),
	} {
		got, err := f()
		if err != nil {
			t.Errorf("%s: unexpected error: %v", name, err)
		}
		if len(got) != 0 {
			t.Errorf("%s: expected an empty frame, got %q", name, got)
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

func TestProgress_CurrentEqualsTotalIsValidNotAnError(t *testing.T) {
	f := Progress(func() (int, int) { return 10, 10 }, func(current, total int) []byte {
		return []byte("done")
	})

	got, err := f()
	if err != nil {
		t.Fatalf("unexpected error at current == total: %v", err)
	}
	if string(got) != "done" {
		t.Errorf("expected %q, got %q", "done", got)
	}
}

func TestJoin_NoNilsFastPathAliasesInputSlice(t *testing.T) {
	fs := []FrameFunc{Static("a"), Static("b")}
	f := Join(" ", fs...)

	fs[0] = Static("replaced")

	got, err := f()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "replaced b" {
		t.Errorf("expected the fast path to alias fs (observing the post-construction replacement), got %q", got)
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

func TestDefaultDurationFormat_RendersSecondsWithOneDecimal(t *testing.T) {
	format := DefaultDurationFormat()

	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{0, "0.0s"},
		{2300 * time.Millisecond, "2.3s"},
		{10 * time.Second, "10.0s"},
	} {
		if got := format(tc.d); got != tc.want {
			t.Errorf("format(%v): got %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestDefaultDurationOptions_FieldsAreSane(t *testing.T) {
	opt := DefaultDurationOptions()

	if opt.StartAt != nil {
		t.Errorf("expected a nil default StartAt (captured lazily on first call), got %v", opt.StartAt)
	}
	if opt.Format == nil {
		t.Fatal("expected a non-nil default Format")
	}
	if got, want := opt.Format(2300*time.Millisecond), "2.3s"; got != want {
		t.Errorf("expected default Format to match DefaultDurationFormat, got %q, want %q", got, want)
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

func TestDuration_NilOptionsFuncInSliceIsSkippedWithoutPanic(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := base
	f := Duration(func() time.Time { return clock }, nil, DurationWithFormat(func(d time.Duration) string {
		return fmt.Sprintf("elapsed=%dms", d.Milliseconds())
	}), nil)

	if _, err := f(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	clock = base.Add(150 * time.Millisecond)
	got, err := f()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "elapsed=150ms" {
		t.Errorf("expected a nil DurationOptionsFunc to be skipped and the real option after it still applied, got %q", got)
	}
}

func TestDuration_ExplicitlyNilFormatFromCustomOptionsFuncReturnsNoop(t *testing.T) {
	nilOutFormat := func(do DurationOptions) DurationOptions {
		do.Format = nil
		return do
	}

	got, err := Duration(time.Now, nilOutFormat)()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected an empty frame from Noop, got %q", got)
	}
}

func TestJoin_AllNilReturnsNoop(t *testing.T) {
	got, err := Join(" ", nil, nil)()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected an empty frame from Noop, got %q", got)
	}
}

func TestJoin_AllSegmentsErrorReturnsErrNoFrame(t *testing.T) {
	f := Join(" ", errorFrame(errors.New("boom 1")), errorFrame(errors.New("boom 2")))

	_, err := f()
	if !errors.Is(err, ErrNoFrame) {
		t.Errorf("expected ErrNoFrame when every segment errors on a call, got %v", err)
	}
}

func TestDuration_NilFormatDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Duration(timer, DurationWithFormat(nil))() panicked: %v", r)
		}
	}()
	f := Duration(time.Now, DurationWithFormat(nil))
	_, _ = f()
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

	f := Duration(timer, DurationWithStartAt(explicitStart))
	if timerCalls != 0 {
		t.Errorf("expected DurationWithStartAt to skip the construction-time timer() call, but timer was called %d time(s)", timerCalls)
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

	f := Duration(func() time.Time { return clock }, DurationWithStartAt(future))

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

func TestRandom_NilRandDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Random(states, nil)() panicked: %v", r)
		}
	}()
	f := Random([]string{"a", "b", "c"}, nil)
	_, _ = f()
}

func TestRandom_SkipsLeadingNilsAndUsesFirstNonNil(t *testing.T) {
	states := []string{"a", "b", "c", "d"}
	real := rand.New(rand.NewPCG(9, 9))
	f := Random(states, nil, real)
	mirror := rand.New(rand.NewPCG(9, 9))

	for i := range 10 {
		got, err := f()
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		want := states[mirror.IntN(len(states))]
		if string(got) != want {
			t.Errorf("call %d: got %q, want %q — a leading nil should be skipped in favor of the first non-nil *rand.Rand", i, got, want)
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

func TestRandomOnceEvery_NilRandDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RandomOnceEvery(states, mod, nil) panicked: %v", r)
		}
	}()
	_ = RandomOnceEvery([]string{"a", "b", "c"}, 3, nil)
}

func TestRandomOnceEvery_SkipsLeadingNilsAndUsesFirstNonNil(t *testing.T) {
	const mod = 3
	states := []string{"a", "b", "c", "d", "e"}
	newFn := func(rands ...*rand.Rand) FrameFunc {
		return RandomOnceEvery(states, mod, rands...)
	}
	f1 := newFn(nil, rand.New(rand.NewPCG(123, 456)))
	f2 := newFn(rand.New(rand.NewPCG(123, 456)))

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("RandomOnceEvery(states, mod, nil, real)() panicked on redraw: %v", r)
			}
		}()
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
				t.Fatalf("call %d: leading nil changed the draw sequence: %q vs %q — it should be skipped in favor of the first non-nil *rand.Rand", i, g1, g2)
			}
		}
	}()
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

func TestDynamic_NilGetWidthFallsBackToNoop(t *testing.T) {
	var got []byte
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Dynamic(nil, build)() panicked: %v", r)
			}
		}()
		got, err = Dynamic(nil, func(width int) FrameFunc { return Static("x") })()
	}()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected a nil getWidth to fall back to Noop, got %q", got)
	}
}

func TestDynamic_NilBuildFallsBackToNoop(t *testing.T) {
	var got []byte
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Dynamic(getWidth, nil)() panicked: %v", r)
			}
		}()
		got, err = Dynamic(func() int { return 40 }, nil)()
	}()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected a nil build to fall back to Noop, got %q", got)
	}
}

func TestDynamic_UsesInitialWidthUpfront(t *testing.T) {
	var gotWidth int
	f := Dynamic(func() int { return 40 }, func(width int) FrameFunc {
		gotWidth = width
		return Static("x")
	})

	if _, err := f(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotWidth != 40 {
		t.Errorf("expected build to be called with the initial width 40, got %d", gotWidth)
	}
}

func TestDynamic_DoesNotRebuildWithoutAWidthChange(t *testing.T) {
	builds := 0
	f := Dynamic(func() int { return 40 }, func(width int) FrameFunc {
		builds++
		return Static("x")
	})

	for range 5 {
		if _, err := f(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if builds != 1 {
		t.Errorf("expected build to be called exactly once across unchanged-width calls, got %d", builds)
	}
}

func TestDynamic_RebuildsOnWidthChange(t *testing.T) {
	width := 40
	builds := []int{}
	f := Dynamic(func() int { return width }, func(w int) FrameFunc {
		builds = append(builds, w)
		return Static("x")
	})

	if _, err := f(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	width = 80
	if _, err := f(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	width = 80
	if _, err := f(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []int{40, 80}
	if !slices.Equal(builds, want) {
		t.Errorf("expected build calls %v, got %v", want, builds)
	}
}

func TestDynamic_InnerFrameFuncKeepsAdvancingBetweenRebuilds(t *testing.T) {
	f := Dynamic(func() int { return 40 }, func(width int) FrameFunc {
		return Simple([]string{"a", "b", "c"})
	})

	var got []string
	for range 4 {
		frame, err := f()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got = append(got, string(frame))
	}

	want := []string{"a", "b", "c", "a"}
	if !slices.Equal(got, want) {
		t.Errorf("expected the inner FrameFunc to keep advancing across calls, got %v want %v", got, want)
	}
}

func TestDynamic_PropagatesInnerFrameFuncError(t *testing.T) {
	wantErr := errors.New("boom")
	f := Dynamic(func() int { return 40 }, func(width int) FrameFunc {
		return errorFrame(wantErr)
	})

	if _, err := f(); !errors.Is(err, wantErr) {
		t.Errorf("expected error %v, got %v", wantErr, err)
	}
}

func TestCropToWidth_NilSourceYieldsNoop(t *testing.T) {
	var got []byte
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("CropToWidth(nil)(40)() panicked: %v", r)
			}
		}()
		got, err = CropToWidth(nil)(40)()
	}()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected a nil source to fall back to Noop, got %q", got)
	}
}

func TestCropToWidth_CropsPlainTextToWidth(t *testing.T) {
	got, err := CropToWidth(staticFrame([]byte("hello world")))(5)()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

func TestCropToWidth_WidthCoveringTheWholeFrameLeavesItUnchanged(t *testing.T) {
	const frame = "hello"
	got, err := CropToWidth(staticFrame([]byte(frame)))(80)()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != frame {
		t.Errorf("expected the frame unchanged (%q), got %q", frame, got)
	}
}

func TestCropToWidth_CountsDisplayCellsNotBytes(t *testing.T) {
	// Each CJK glyph is two cells: width 5 fits 你好 (4) but not 世 (would be 6).
	got, err := CropToWidth(staticFrame([]byte("你好世界")))(5)()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "你好" {
		t.Errorf("expected %q, got %q", "你好", got)
	}
}

func TestCropToWidth_KeepsZeroWidthSequencesPastTheLimit(t *testing.T) {
	got, err := CropToWidth(staticFrame([]byte("\033[31mhello\033[0m")))(3)()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w := graphemeOpts.String(string(got)); w != 3 {
		t.Errorf("expected 3 visible cells, got %d (%q)", w, got)
	}
	if !strings.HasSuffix(string(got), "\033[0m") {
		t.Errorf("expected the trailing reset to survive the crop, got %q", got)
	}
}

func TestCropToWidth_NonPositiveWidthLeavesOnlyZeroWidthContent(t *testing.T) {
	for _, w := range []int{0, -5} {
		got, err := CropToWidth(staticFrame([]byte("\033[31mhi\033[0m")))(w)()
		if err != nil {
			t.Fatalf("width %d: unexpected error: %v", w, err)
		}
		if vis := graphemeOpts.String(string(got)); vis != 0 {
			t.Errorf("width %d: expected 0 visible cells, got %d (%q)", w, vis, got)
		}
		if string(got) != "\033[31m\033[0m" {
			t.Errorf("width %d: expected both escapes preserved, got %q", w, got)
		}
	}
}

func TestCropToWidth_PropagatesSourceError(t *testing.T) {
	wantErr := errors.New("boom")
	if _, err := CropToWidth(errorFrame(wantErr))(10)(); !errors.Is(err, wantErr) {
		t.Errorf("expected error %v, got %v", wantErr, err)
	}
}

func TestCropToWidth_ReCropsAsWidthChangesUnderDynamic(t *testing.T) {
	width := 3
	f := Dynamic(func() int { return width }, CropToWidth(staticFrame([]byte("abcdefgh"))))

	got, err := f()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "abc" {
		t.Errorf("width 3: expected %q, got %q", "abc", got)
	}

	width = 6
	got, err = f()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "abcdef" {
		t.Errorf("width 6: expected %q, got %q", "abcdef", got)
	}
}
