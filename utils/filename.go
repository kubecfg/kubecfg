package utils

import (
	"fmt"
	"regexp"
	"strings"
)

const lowerHex = "0123456789abcdef"

var allChars = regexp.MustCompile(`.`)

// Symbols that are never allowed in file names on at least one platform
// Note that we include '%', which is allowed, but we need to use it as an escape character
var illegalSymbols = regexp.MustCompile(`[?<>*|\\/%:"^[:cntrl:]]`)

// These are reserved words and symbols which cannot be a file name, with or without a file extension
// Mainly special Windows devices
// See https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file#naming-conventions
var reservedWords = regexp.MustCompile(`^aux|^com[0-9¹²³]|^con|^lpt[0-9¹²³]|^nul|^prn`)

// Symbols that cannot (or should not) end a file name
// Windows does not allow file names to end in a '.' or ' '
var illegalSuffix = regexp.MustCompile(`[. ]$`)

// Encode percent encodes a filename string, to minimize cross-platform compatibility issues related to case handling,
// allowed characters, and reserved words
func Encode(str string) (string, error) {
	if len(str) == 0 {
		return str, nil
	}
	var filename = str
	filename = illegalSymbols.ReplaceAllStringFunc(filename, escapeAll)
	filename = allChars.ReplaceAllStringFunc(filename, escapeUpper)
	filename = illegalSuffix.ReplaceAllStringFunc(filename, escapeAll)

	parts := strings.SplitN(filename, ".", 2)
	parts[0] = reservedWords.ReplaceAllStringFunc(parts[0], escapeAll)

	return strings.Join(parts, "."), nil
}

// escapeAll converts every byte in string s to a percent encoded string
// similar to net/url methods, but simplified to remove any decision logic, and using lowercase letters
func escapeAll(s string) string {
	var buf strings.Builder

	for i := 0; i < len(s); i++ {
		buf.WriteString(fmt.Sprintf("%%%02x", s[i]))
	}

	return buf.String()
}

func escapeUpper(s string) string {
	if s == strings.ToLower(s) {
		return s
	}
	return escapeAll(s)
}
