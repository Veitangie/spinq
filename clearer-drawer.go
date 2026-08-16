// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

package spinq

import (
	"bytes"

	"github.com/clipperhouse/displaywidth"
	"github.com/clipperhouse/uax29/v2/graphemes"
	"veitangie.dev/spinq/internal/stripansi"
)

const lineUp = "\033[A"

var clearPrevLine = append([]byte(lineUp), clearBytes...)

type clearerDrawer interface {
	clear(*spinnerState) error
	draw(*spinnerState) error
	adjust(*spinnerState)
}

type obliviousClearerDrawer struct{}

var _ clearerDrawer = obliviousClearerDrawer{}

// NOT THREAD SAFE
func (obliviousClearerDrawer) clear(st *spinnerState) error {
	if st.needClear {
		st.needClear = false
		_, err := st.wrapped.Write(clearBytes)
		return err
	}
	return nil
}

// NOT THREAD SAFE
func (obliviousClearerDrawer) draw(st *spinnerState) error {
	if st.running.Load() && st.canWrite && !st.needClear && len(st.frame) != 0 {
		st.needClear = true
		_, err := st.wrapped.Write(st.frame)
		return err
	}
	return nil
}

func (obliviousClearerDrawer) adjust(*spinnerState) {}

type awareClearerDrawer struct {
	width    int
	getWidth func() int
	visible  []byte
}

var _ clearerDrawer = &awareClearerDrawer{}

// NOT THREAD SAFE
func (a *awareClearerDrawer) clear(st *spinnerState) error {
	err := a.handleResize(st)
	if err != nil {
		return err
	}

	if st.needClear {
		st.needClear = false
		_, err := st.wrapped.Write(clearBytes)
		return err
	}
	return nil
}

// NOT THREAD SAFE
func (a *awareClearerDrawer) draw(st *spinnerState) error {
	if st.running.Load() && st.canWrite && !st.needClear && len(st.frame) != 0 && a.width > 0 {
		if len(a.visible) == 0 {
			a.adjust(st)
		}
		st.needClear = true
		_, err := st.wrapped.Write(a.visible)
		return err
	}
	return nil
}

// NOT THREAD SAFE
func (a *awareClearerDrawer) handleResize(st *spinnerState) error {
	newWidth := a.getWidth()
	if newWidth == a.width {
		return nil
	}

	a.width = newWidth
	err := a.clearMess(st)
	if err != nil {
		return err
	}

	a.adjust(st)
	return nil
}

// NOT THREAD SAFE
func (a *awareClearerDrawer) clearMess(st *spinnerState) error {
	if !st.needClear || a.width == 0 {
		return nil
	}

	linesToClear := max((displaywidth.Bytes(stripansi.StripBytes(a.visible))+a.width-1)/a.width, 1)
	totalSeq := bytes.Buffer{}
	for curLine := range linesToClear {
		if curLine == 0 {
			_, _ = totalSeq.Write(clearBytes)
		} else {
			_, _ = totalSeq.Write(clearPrevLine)
		}
	}
	st.needClear = false
	_, err := st.wrapped.Write(totalSeq.Bytes())
	return err
}

// NOT THREAD SAFE
func (a *awareClearerDrawer) adjust(st *spinnerState) {
	if a.width >= len(st.frame) || a.width >= displaywidth.Bytes(stripansi.StripBytes(st.frame)) {
		a.visible = st.frame
		return
	}

	iter := graphemes.FromBytes(st.frame)
	iter.AnsiEscapeSequences = true
	iter.AnsiEscapeSequences8Bit = true
	total := 0
	canTakeMore := true
	result := bytes.Buffer{}
	for iter.Next() {
		cur := iter.Value()
		if len(cur) == 0 || cur[0] == 0o33 {
			result.Write(cur)
			continue
		}
		if !canTakeMore {
			continue
		}

		size := displaywidth.Bytes(cur)
		if total+size <= a.width {
			total += size
			result.Write(cur)
			continue
		}
		canTakeMore = false
	}

	a.visible = result.Bytes()
}
