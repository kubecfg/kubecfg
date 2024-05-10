package utils

import (
	"regexp"
	"strings"
)

const lowerHex = "0123456789abcdef"

var allowedChars = regexp.MustCompile(`[^0-9a-z!@#$()\[\]{},.;~\-_=+ ]`)

// These are reserved words and symbols which cannot be a file name, with or without a file extension
// Mainly special Windows devices
// See https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file#naming-conventions
var reservedWords = regexp.MustCompile(`^aux|^com[0-9¹²³]|^con|^lpt[0-9¹²³]|^nul|^prn`)

// Symbols that cannot (or should not) begin a file name
// This is technically legal on all major OSes, but it's very hard to work with these files because many common shells
// interpret the file name as a flag rather than a file
var illegalPrefix = regexp.MustCompile(`^-`)

// Symbols that cannot (or should not) end a file name
// Windows does not allow file names to end in a '.' or ' '
var illegalSuffix = regexp.MustCompile(`[. ]$`)

// Encode percent encodes a filename string, to minimize cross-platform compatibility issues related to case handling,
// allowed characters, and reserved words
func Encode(str string) (string, error) {
	filename := allowedChars.ReplaceAllStringFunc(str, escape)
	//filename = illegalSolo.ReplaceAllStringFunc(filename, escape)
	filename = illegalPrefix.ReplaceAllStringFunc(filename, escape)
	filename = illegalSuffix.ReplaceAllStringFunc(filename, escape)

	parts := strings.SplitN(filename, ".", 2)
	parts[0] = reservedWords.ReplaceAllStringFunc(parts[0], escape)

	return strings.Join(parts, "."), nil
}

// escape converts every byte in string s to a percent encoded string
// similar to net/url methods, but simplified to remove any decision logic, and using lowercase letters
func escape(s string) string {
	var buf strings.Builder

	for i := 0; i < len(s); i++ {
		c := s[i]
		buf.WriteByte('%')
		buf.WriteByte(lowerHex[c>>4])
		buf.WriteByte(lowerHex[c&15])
	}

	return buf.String()
}
