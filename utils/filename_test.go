package utils

import (
	"net/url"
	"testing"
)

type filenameScenario struct {
	scenario string
	given    string
	expected string
}

var scenarios = []filenameScenario{
	{
		scenario: "empty",
		given:    "",
		expected: "",
	},
	{
		scenario: "no escapes",
		given:    "this_is_valid",
		expected: "this_is_valid",
	},
	{
		scenario: "with extension",
		given:    "this_is_valid.txt",
		expected: "this_is_valid.txt",
	},
	{
		scenario: "with multiple dots",
		given:    "this_is_valid..txt",
		expected: "this_is_valid..txt",
	},
	{
		scenario: "spaces",
		given:    "this has spaces",
		expected: "this has spaces",
	},
	{
		scenario: "capitals",
		given:    "This Has Caps",
		expected: "%54his %48as %43aps",
	},
	{
		scenario: "multi-code point symbols",
		given:    "fist👊🏼bump",
		expected: "fist👊🏼bump",
	},
	{
		scenario: "disallowed symbols",
		given:    `^_"_:_?_<_>_*_|_\_/_%`,
		expected: "%5e_%22_%3a_%3f_%3c_%3e_%2a_%7c_%5c_%2f_%25",
	},
	{
		scenario: "one dot",
		given:    ".",
		expected: "%2e",
	},
	{
		scenario: "two dots",
		given:    "..",
		expected: ".%2e",
	},
	{
		scenario: "all dots",
		given:    "....",
		expected: "...%2e",
	},
	{
		scenario: "reserved words (aux)",
		given:    "aux.ext",
		expected: "%61%75%78.ext",
	},
	{
		scenario: "reserved words (com)",
		given:    "com1.ext",
		expected: "%63%6f%6d%31.ext",
	},
	{
		scenario: "reserved words (con)",
		given:    "con.ext",
		expected: "%63%6f%6e.ext",
	},
	{
		scenario: "reserved words (lpt)",
		given:    "lpt1.ext",
		expected: "%6c%70%74%31.ext",
	},
	{
		scenario: "reserved words (nul)",
		given:    "nul.ext",
		expected: "%6e%75%6c.ext",
	},
	{
		scenario: "reserved words (prn)",
		given:    "prn.ext",
		expected: "%70%72%6e.ext",
	},
	{
		scenario: "ends with .",
		given:    "bad_idea.",
		expected: "bad_idea%2e",
	},
	{
		scenario: "ends with space",
		given:    "invalid ",
		expected: "invalid%20",
	},
	{
		scenario: "Cyrillic capitals",
		given:    "Б б Г г Д д",
		expected: "%d0%91 б %d0%93 г %d0%94 д",
	},
	{
		scenario: "Greek capitals",
		given:    "Γ γ Δ δ Ω ω",
		expected: "%ce%93 γ %ce%94 δ %ce%a9 ω",
	},
}

func TestEncode(t *testing.T) {
	for _, s := range scenarios {
		t.Run(s.scenario, func(t *testing.T) {
			actual, err := Encode(s.given)
			if err != nil {
				t.Error(err)
			}

			if s.expected != actual {
				t.Fatalf("Expected: %s, but actual was: %s", s.expected, actual)
			}
		})
	}
}

func TestUnencode(t *testing.T) {
	for _, s := range scenarios {
		t.Run(s.scenario, func(t *testing.T) {
			encoded, err := Encode(s.given)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := url.PathUnescape(encoded)
			if err != nil {
				t.Fatal(err)
			}

			if s.given != actual {
				t.Fatalf("Expected: %s, but actual was: %s", s.given, actual)
			}
		})
	}
}
