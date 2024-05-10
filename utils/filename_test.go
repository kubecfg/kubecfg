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
		given:    "This Has Caps And Spaces",
		expected: "%54his %48as %43aps %41nd %53paces",
	},
	{
		scenario: "multi-code point symbols",
		given:    "fist👊🏼bump",
		expected: "fist%f0%9f%91%8a%f0%9f%8f%bcbump",
	},
	{
		scenario: "disallowed symbols",
		given:    "?_<_>_*_|_\\_/_%",
		expected: "%3f_%3c_%3e_%2a_%7c_%5c_%2f_%25",
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
		scenario: "starts with -",
		given:    "-why_would_you_do_this",
		expected: "%2dwhy_would_you_do_this",
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
