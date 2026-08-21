// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

package spinq

import (
	"bytes"
	"math"
	"strconv"

	"github.com/clipperhouse/displaywidth"
	"veitangie.dev/spinq/internal/stripansi"
)

// RenderFunc renders a progress bar/counter from a (current, total)
// reading. Callers are contractually responsible for only ever invoking a
// RenderFunc with total > 0 and current <= total - Progress is the one
// place in spinq that enforces this, so a RenderFunc is free to assume it.
type RenderFunc func(int, int) []byte

// NoopRender returns a RenderFunc that always renders nothing.
func NoopRender() RenderFunc {
	return func(_, _ int) []byte { return []byte{} }
}

// Join returns a RenderFunc that renders rf's and other's output joined by
// sep. A nil other returns rf unchanged.
func (rf RenderFunc) Join(sep string, other RenderFunc) RenderFunc {
	if other == nil {
		return rf
	}

	sepBytes := []byte(sep)
	return func(current, total int) []byte {
		rfRes := rf(current, total)
		otherRes := other(current, total)
		res := make([]byte, 0, len(rfRes)+len(sepBytes)+len(otherRes))

		res = append(res, rfRes...)
		res = append(res, sepBytes...)
		res = append(res, otherRes...)
		return res
	}
}

// JoinRender returns a RenderFunc that renders each of fs joined by sep.
// Nil entries in fs are dropped; if every entry is nil, JoinRender returns
// NoopRender.
func JoinRender(sep string, fs ...RenderFunc) RenderFunc {
	nils := 0
	for _, f := range fs {
		if f == nil {
			nils += 1
		}
	}

	var fsNoNils []RenderFunc
	if nils == 0 {
		fsNoNils = fs
	} else {
		fsNoNils = make([]RenderFunc, 0, len(fs)-nils)
		for _, f := range fs {
			if f != nil {
				fsNoNils = append(fsNoNils, f)
			}
		}
	}

	if len(fsNoNils) == 0 {
		return NoopRender()
	}

	sepBytes := []byte(sep)

	return func(current, total int) []byte {
		res := make([][]byte, len(fsNoNils))
		for i, f := range fsNoNils {
			resF := f(current, total)
			res[i] = resF
		}

		return bytes.Join(res, sepBytes)
	}
}

// RenderDirection controls which side of a bar the filled portion grows
// from. Right (the default) fills from the start; Left mirrors it, filling
// from the end instead, with Full/Empty swapped and the divider position
// reflected accordingly.
type RenderDirection bool

const (
	Right RenderDirection = false
	Left  RenderDirection = true
)

// SmoothBarOptions configures SmoothBarRender.
type SmoothBarOptions struct {
	Start     string
	Full      string
	Dividers  []string
	Empty     string
	End       string
	Direction RenderDirection
}

// DefaultSmoothBarOptions returns SmoothBarRender's default options: a
// "[...]"-bracketed bar using Unicode eighths block characters ("▏".."█")
// as its Dividers, for sub-cell-precision fill.
func DefaultSmoothBarOptions() SmoothBarOptions {
	return SmoothBarOptions{
		Start:     "",
		Full:      "█",
		Dividers:  []string{"▏", "▎", "▍", "▌", "▋", "▊", "▉", "█"},
		Empty:     " ",
		End:       "",
		Direction: Right,
	}
}

// SmoothBarOptionsFunc configures a SmoothBarOptions value.
type SmoothBarOptionsFunc func(SmoothBarOptions) SmoothBarOptions

// SmoothWithStart sets the bar's leading string, e.g. "[".
func SmoothWithStart(start string) SmoothBarOptionsFunc {
	return func(bo SmoothBarOptions) SmoothBarOptions {
		bo.Start = start
		return bo
	}
}

// SmoothWithFull sets the glyph used for a fully-filled cell.
func SmoothWithFull(full string) SmoothBarOptionsFunc {
	return func(bo SmoothBarOptions) SmoothBarOptions {
		bo.Full = full
		return bo
	}
}

// SmoothWithDivider sets the ordered set of sub-cell glyphs used at the
// boundary between filled and empty, from emptiest to fullest. All of them,
// plus Full and Empty, must render at the same width, or SmoothBarRender
// returns NoopRender. Fewer than two dividers falls back to BarRender: zero
// dividers uses BarRender's own default divider, one divider is used as
// BarRender's fixed divider.
func SmoothWithDivider(dividers []string) SmoothBarOptionsFunc {
	return func(bo SmoothBarOptions) SmoothBarOptions {
		bo.Dividers = dividers
		return bo
	}
}

// SmoothWithEmpty sets the glyph used for a fully-empty cell.
func SmoothWithEmpty(empty string) SmoothBarOptionsFunc {
	return func(bo SmoothBarOptions) SmoothBarOptions {
		bo.Empty = empty
		return bo
	}
}

// SmoothWithEnd sets the bar's trailing string, e.g. "]".
func SmoothWithEnd(end string) SmoothBarOptionsFunc {
	return func(bo SmoothBarOptions) SmoothBarOptions {
		bo.End = end
		return bo
	}
}

// SmoothWithDirection sets which side of the bar fills first; see
// RenderDirection.
func SmoothWithDirection(direction RenderDirection) SmoothBarOptionsFunc {
	return func(bo SmoothBarOptions) SmoothBarOptions {
		bo.Direction = direction
		return bo
	}
}

// WithSmoothOptions replaces the entire SmoothBarOptions with opt,
// discarding any options applied earlier in the same SmoothBarRender call.
func WithSmoothOptions(opt SmoothBarOptions) SmoothBarOptionsFunc {
	return func(bo SmoothBarOptions) SmoothBarOptions {
		return opt
	}
}

// BarOptions configures BarRender.
type BarOptions struct {
	Start     string
	Full      string
	Divider   string
	Empty     string
	End       string
	Direction RenderDirection
}

// DefaultBarOptions returns BarRender's default options: "[===>   ]".
func DefaultBarOptions() BarOptions {
	return BarOptions{
		Start:     "[",
		Full:      "=",
		Divider:   ">",
		Empty:     " ",
		End:       "]",
		Direction: Right,
	}
}

// WithRoundedBarOptions returns a preset BarOptions styled as "(###>----)".
// Like WithBarOptions, it replaces the whole BarOptions - apply it before
// any BarWith* tweaks in the same BarRender call, not after, or it discards
// them.
func WithRoundedBarOptions() BarOptionsFunc {
	return func(bo BarOptions) BarOptions {
		return BarOptions{
			Start:     "(",
			Full:      "#",
			Divider:   ">",
			Empty:     "-",
			End:       ")",
			Direction: Right,
		}
	}
}

// WithShadeBarOptions returns a preset BarOptions styled as "███░░░", using
// shaded block characters with no brackets. Like WithBarOptions, it replaces
// the whole BarOptions - apply it before any BarWith* tweaks in the same
// BarRender call, not after, or it discards them.
func WithShadeBarOptions() BarOptionsFunc {
	return func(bo BarOptions) BarOptions {
		return BarOptions{
			Start:     "",
			Full:      "█",
			Divider:   "█",
			Empty:     "░",
			End:       "",
			Direction: Right,
		}
	}
}

// WithDotBarOptions returns a preset BarOptions styled as "(●●●○○○)". Like
// WithBarOptions, it replaces the whole BarOptions - apply it before any
// BarWith* tweaks in the same BarRender call, not after, or it discards
// them.
func WithDotBarOptions() BarOptionsFunc {
	return func(bo BarOptions) BarOptions {
		return BarOptions{
			Start:     "(",
			Full:      "●",
			Divider:   "●",
			Empty:     "○",
			End:       ")",
			Direction: Right,
		}
	}
}

// WithMinimalBarOptions returns a preset BarOptions styled as "###>---", with
// no brackets. Like WithBarOptions, it replaces the whole BarOptions - apply
// it before any BarWith* tweaks in the same BarRender call, not after, or it
// discards them.
func WithMinimalBarOptions() BarOptionsFunc {
	return func(bo BarOptions) BarOptions {
		return BarOptions{
			Start:     "",
			Full:      "#",
			Divider:   ">",
			Empty:     "-",
			End:       "",
			Direction: Right,
		}
	}
}

// WithThinBarOptions returns a preset BarOptions styled as "▰▰▰▱▱▱", with no
// brackets. Like WithBarOptions, it replaces the whole BarOptions - apply it
// before any BarWith* tweaks in the same BarRender call, not after, or it
// discards them.
func WithThinBarOptions() BarOptionsFunc {
	return func(bo BarOptions) BarOptions {
		return BarOptions{
			Start:     "",
			Full:      "▰",
			Divider:   "▰",
			Empty:     "▱",
			End:       "",
			Direction: Right,
		}
	}
}

// BarOptionsFunc configures a BarOptions value.
type BarOptionsFunc func(BarOptions) BarOptions

// BarWithStart sets the bar's leading string, e.g. "[".
func BarWithStart(start string) BarOptionsFunc {
	return func(bo BarOptions) BarOptions {
		bo.Start = start
		return bo
	}
}

// BarWithFull sets the glyph used for a filled cell.
func BarWithFull(full string) BarOptionsFunc {
	return func(bo BarOptions) BarOptions {
		bo.Full = full
		return bo
	}
}

// BarWithDivider sets the single glyph drawn at the boundary between filled
// and empty.
func BarWithDivider(divider string) BarOptionsFunc {
	return func(bo BarOptions) BarOptions {
		bo.Divider = divider
		return bo
	}
}

// BarWithEmpty sets the glyph used for an empty cell. It must render at the
// same width as Full, or BarRender returns NoopRender.
func BarWithEmpty(empty string) BarOptionsFunc {
	return func(bo BarOptions) BarOptions {
		bo.Empty = empty
		return bo
	}
}

// BarWithEnd sets the bar's trailing string, e.g. "]".
func BarWithEnd(end string) BarOptionsFunc {
	return func(bo BarOptions) BarOptions {
		bo.End = end
		return bo
	}
}

// BarWithDirection sets which side of the bar fills first; see
// RenderDirection.
func BarWithDirection(direction RenderDirection) BarOptionsFunc {
	return func(bo BarOptions) BarOptions {
		bo.Direction = direction
		return bo
	}
}

// WithBarOptions replaces the entire BarOptions with opt, discarding any
// options applied earlier in the same BarRender call.
func WithBarOptions(opt BarOptions) BarOptionsFunc {
	return func(bo BarOptions) BarOptions {
		return opt
	}
}

// SmoothBarRender returns a RenderFunc that draws a progress bar length
// cells wide, using sub-cell-precision divider glyphs (see
// SmoothWithDivider) at the boundary between filled and empty for smoother
// visual movement than BarRender's single fixed divider. It renders at a
// constant width across every progress level except one deliberate
// exception: at exactly 100% (or 0% with Direction: Left), the boundary
// divider is dropped in favor of a plain Full/Empty cell, since forcing a
// mismatched divider glyph there would look more glitchy, not less.
//
// It returns NoopRender if length leaves no room for the bar, or if the
// configured glyphs don't all render at a consistent width.
func SmoothBarRender(length int, opts ...SmoothBarOptionsFunc) RenderFunc {
	opt := DefaultSmoothBarOptions()
	for _, f := range opts {
		opt = f(opt)
	}

	if len(opt.Dividers) < 2 {
		optStandard := BarOptions{}
		optStandard.Start = opt.Start
		optStandard.Full = opt.Full
		optStandard.Empty = opt.Empty
		optStandard.End = opt.End
		optStandard.Direction = opt.Direction
		if len(opt.Dividers) == 1 {
			optStandard.Divider = opt.Dividers[0]
		}
		return BarRender(length, WithBarOptions(optStandard))
	}

	if opt.Direction {
		opt.Full, opt.Empty = opt.Empty, opt.Full
	}

	divLength := displaywidth.String(stripansi.Strip(opt.Dividers[0]))
	for _, div := range opt.Dividers[1:] {
		if divLength != displaywidth.String(stripansi.Strip(div)) {
			return NoopRender()
		}
	}
	constPartLength := displaywidth.String(stripansi.Strip(opt.Start)) +
		displaywidth.String(stripansi.Strip(opt.End))
	length -= constPartLength
	if length <= 0 {
		return NoopRender()
	}

	unitLength := displaywidth.String(stripansi.Strip(opt.Empty))
	if unitLength != displaywidth.String(stripansi.Strip(opt.Full)) || unitLength != divLength || unitLength > length {
		return NoopRender()
	}

	lengthInUnits := float64(length) / float64(unitLength)

	return func(current, total int) []byte {
		naiveScale := lengthInUnits / float64(total)
		scaledDone := int(float64(current) * naiveScale)
		divPart := float64(current) - float64(scaledDone)/naiveScale
		divPartOfUnit := divPart * naiveScale
		divIdx := len(opt.Dividers) - 1
		if current < total {
			divIdx = int(math.Floor(float64(len(opt.Dividers)) * divPartOfUnit))
			divIdx = max(divIdx, 0)
			divIdx = min(divIdx, len(opt.Dividers)-1)
		}
		divider := opt.Dividers[divIdx]
		if opt.Direction {
			scaledDone = int(lengthInUnits) - scaledDone - 1
		}

		buf := bytes.NewBuffer(make([]byte, 0, length+constPartLength))
		buf.WriteString(opt.Start)
		for idx := range int(lengthInUnits) {
			if idx < scaledDone {
				buf.WriteString(opt.Full)
				continue
			}
			if idx == scaledDone {
				buf.WriteString(divider)
				continue
			}
			buf.WriteString(opt.Empty)
		}
		buf.WriteString(opt.End)

		return buf.Bytes()
	}
}

// BarRender returns a RenderFunc that draws a progress bar length cells
// wide, with a single fixed divider glyph at the boundary between filled
// and empty (see BarWithDivider). It renders at a constant width across
// every progress level, including 100%.
//
// It returns NoopRender if length leaves no room for the bar, or if Full
// and Empty don't render at the same width.
func BarRender(length int, opts ...BarOptionsFunc) RenderFunc {
	opt := DefaultBarOptions()
	for _, f := range opts {
		opt = f(opt)
	}

	if opt.Direction {
		opt.Full, opt.Empty = opt.Empty, opt.Full
	}

	constPartLength := displaywidth.String(stripansi.Strip(opt.Start)) +
		displaywidth.String(stripansi.Strip(opt.Divider)) +
		displaywidth.String(stripansi.Strip(opt.End))
	length -= constPartLength
	if length <= 0 {
		return NoopRender()
	}

	unitLength := displaywidth.String(stripansi.Strip(opt.Empty))
	if unitLength != displaywidth.String(stripansi.Strip(opt.Full)) || unitLength > length {
		return NoopRender()
	}

	lengthInUnits := float64(length) / float64(unitLength)

	return func(current, total int) []byte {
		naiveScale := lengthInUnits / float64(total)
		scaledDone := int(float64(current) * naiveScale)
		if opt.Direction {
			scaledDone = int(lengthInUnits) - scaledDone
		}

		buf := bytes.NewBuffer(make([]byte, 0, length+constPartLength))
		buf.WriteString(opt.Start)
		for idx := range int(lengthInUnits) {
			if idx < scaledDone {
				buf.WriteString(opt.Full)
				continue
			}
			if idx == scaledDone {
				buf.WriteString(opt.Divider)
			}
			buf.WriteString(opt.Empty)
		}
		if scaledDone == int(lengthInUnits) {
			buf.WriteString(opt.Divider)
		}
		buf.WriteString(opt.End)

		return buf.Bytes()
	}
}

// FractRender returns a RenderFunc that renders "current<sep>total", e.g.
// FractRender("/") renders "5/10".
func FractRender(sep string) RenderFunc {
	sepBytes := []byte(sep)
	return func(current, total int) []byte {
		currentBytes := make([]byte, 0, 20)
		currentBytes = strconv.AppendInt(currentBytes, int64(current), 10)
		totalBytes := make([]byte, 0, 20)
		totalBytes = strconv.AppendInt(totalBytes, int64(total), 10)
		buf := make([]byte, 0, (len(totalBytes)*2)+len(sepBytes))
		for range len(totalBytes) - len(currentBytes) {
			buf = append(buf, ' ')
		}
		buf = append(buf, currentBytes...)
		buf = append(buf, sepBytes...)
		buf = append(buf, totalBytes...)
		return buf
	}
}

// PercentRender returns a RenderFunc that renders current/total as a whole
// percentage, e.g. "50%". It truncates rather than rounds.
func PercentRender() RenderFunc {
	return func(current, total int) []byte {
		percent := int64((float64(current) / float64(total)) * 100)
		buf := make([]byte, 0, 4)
		if percent < 100 {
			buf = append(buf, ' ')
		}
		if percent < 10 {
			buf = append(buf, ' ')
		}
		buf = strconv.AppendInt(buf, percent, 10)
		buf = append(buf, '%')
		return buf
	}
}

// RenderWidthFunc builds a fresh RenderFunc for a given terminal width - see
// DynamicRender. BarRender/SmoothBarRender's length parameter is usually
// what a RenderWidthFunc closes over to produce a correctly-sized render.
type RenderWidthFunc func(int) RenderFunc

// DynamicRender returns a RenderFunc that rebuilds itself via build whenever
// getWidth's value changes - the RenderFunc-level counterpart to Dynamic. It
// slots into Progress/JoinRender/RenderFunc.Join exactly like any other
// RenderFunc, so a width-reactive bar still composes with FractRender,
// PercentRender, and friends the same way a fixed-width one does.
//
// getWidth is called on every call to the returned RenderFunc - once per
// frame render, which spinq treats as a hot path. Always pass the func
// CachedGetWidth returns here, never a raw syscall-backed getWidth directly -
// see Dynamic's doc comment for why.
func DynamicRender(getWidth func() int, build RenderWidthFunc) RenderFunc {
	width := getWidth()
	currentRender := build(width)

	return func(current, total int) []byte {
		newWidth := getWidth()
		if newWidth != width {
			width = newWidth
			currentRender = build(width)
		}
		return currentRender(current, total)
	}
}

// DynamicBarRender is BarRender sized by getWidth instead of a fixed length -
// DynamicRender applied to BarRender. getWidth is called on every render, so
// pass CachedGetWidth's output, not a raw one; shape it first with
// Offset/Portion/Clamp if you want a fraction of the terminal, room reserved
// for fixed-width siblings, or a bounded range, rather than the raw width
// verbatim.
func DynamicBarRender(getWidth func() int, opts ...BarOptionsFunc) RenderFunc {
	return DynamicRender(getWidth, func(width int) RenderFunc {
		return BarRender(width, opts...)
	})
}

// DynamicSmoothBarRender is SmoothBarRender sized by getWidth instead of a
// fixed length - see DynamicBarRender.
func DynamicSmoothBarRender(getWidth func() int, opts ...SmoothBarOptionsFunc) RenderFunc {
	return DynamicRender(getWidth, func(width int) RenderFunc {
		return SmoothBarRender(width, opts...)
	})
}
