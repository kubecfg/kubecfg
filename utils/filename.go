package utils

import (
	"regexp"
	"strings"
)

const lowerHex = "0123456789abcdef"

var allowedChars = regexp.MustCompile(`[^0-9a-z!@#$&()\[\]{},.;~\-_=+]`)

// These are reserved words and symbols which cannot be a file name, with or without a file extension
// Mainly special Windows devices
var reservedWords = regexp.MustCompile(`^aux|^com[0-9¹²³]|^con|^lpt[0-9¹²³]|^nul|^prn`)

// Symbols that cannot (or should not) make up the entirety of a filename
var illegalSolo = regexp.MustCompile(`^\.+$`)

// Symbols that cannot (or should not) begin a file name
var illegalPrefix = regexp.MustCompile(`^-`)

// Symbols that cannot (or should not) end a file name
var illegalSuffix = regexp.MustCompile(`\.$`)

// Encode percent encodes a filename string, to minimize cross-platform compatibility issues related to case handling,
// allowed characters, and reserved words
func Encode(str string) (string, error) {
	filename := allowedChars.ReplaceAllStringFunc(str, escape)
	filename = illegalSolo.ReplaceAllStringFunc(filename, escape)
	filename = illegalPrefix.ReplaceAllStringFunc(filename, escape)
	filename = illegalSuffix.ReplaceAllStringFunc(filename, escape)

	parts := strings.SplitN(filename, ".", 2)
	parts[0] = reservedWords.ReplaceAllStringFunc(parts[0], escape)

	return strings.Join(parts, "."), nil
}

// escape converts every byte in string s to a percent encoded string
// similar to net/url methods, but simplified to remove any decision logic, and using lowercase letters
func escape(s string) string {
	bufSize := len(s)

	if bufSize == 0 {
		return s
	}

	required := len(s) + 2*bufSize
	buf := make([]byte, required)

	for iBuf, i := 0, 0; i < len(s); i++ {
		c := s[i]
		buf[iBuf] = '%'
		buf[iBuf+1] = lowerHex[c>>4]
		buf[iBuf+2] = lowerHex[c&15]
		iBuf += 3
	}
	return string(buf)
}
