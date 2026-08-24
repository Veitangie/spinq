// Copyright (c) 2018 Andrew Carlson
// SPDX-License-Identifier: MIT
package spinq

import (
	"regexp"
)

const ansi = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"

var re = regexp.MustCompile(ansi)

// StripANSI removes ANSI/VT100 escape sequences from str - color codes,
// cursor movement, and other CSI/OSC control sequences - leaving only the
// visible characters. A string with no escape sequences is returned
// unchanged. See StripANSIBytes for the []byte form.
func StripANSI(str string) string {
	return re.ReplaceAllString(str, "")
}

// StripANSIBytes is StripANSI for a []byte instead of a string.
func StripANSIBytes(source []byte) []byte {
	return re.ReplaceAll(source, []byte{})
}
