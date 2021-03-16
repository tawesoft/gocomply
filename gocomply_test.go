package main

import (
	"testing"
)

func TestParseGoImport(t *testing.T) {
	type row struct {
		input      string
		expected   GoImport
		expectedOK bool
	}
	tests := []row{
		{
			// ending in ">"
			input: `<html><meta name="go-import" content="example.org/vanity git https://github.com/example/vanity"></html>`,
			expected: GoImport{
				ImportPrefix: "example.org/vanity",
				Vcs:          "git",
				RepoRoot:     "https://github.com/example/vanity",
			},
			expectedOK: true,
		},
		{
			// ending in "/>"
			input: `<html><meta name="go-import" content="example.org/vanity git https://github.com/example/vanity" /></html>`,
			expected: GoImport{
				ImportPrefix: "example.org/vanity",
				Vcs:          "git",
				RepoRoot:     "https://github.com/example/vanity",
			},
			expectedOK: true,
		},
		{
			// unicode
			input: `<html><meta name="go-import" content="example.org/本 git https://github.com/example/本" /></html>`,
			expected: GoImport{
				ImportPrefix: "example.org/本",
				Vcs:          "git",
				RepoRoot:     "https://github.com/example/本",
			},
			expectedOK: true,
		},
		{
			// awkward whitespace but still valid
			input: `<html>
    <  MeTa  NaMe  =  "go-import"
        CoNtEnT  =  "example.org/foo git https://github.com/example/foo"  /></html>`,
			expected: GoImport{
				ImportPrefix: "example.org/foo",
				Vcs:          "git",
				RepoRoot:     "https://github.com/example/foo",
			},
			expectedOK: true,
		},
	}

	for i, test := range tests {
		gi, ok := parseGoImport(test.input)
		if ok != test.expectedOK {
			t.Errorf("test %d failed: parse error", i)
		} else if gi != test.expected {
			t.Errorf("test %d failed: expected %+v but got %+v",
				i, test.expected, gi)
		}
	}
}

func TestParseGoSource(t *testing.T) {
	type row struct {
		input      string
		expected   GoSource
		expectedOK bool
	}
	tests := []row{
		{
			// real-world example
			input: `<html><meta name="go-source" content="a b c d"></html>`,
			expected: GoSource{
				ImportPrefix: "a",
				Home:         "b",
				Directory:    "c",
				File:         "d",
			},
			expectedOK: true,
		},
		{
			// real-world example
			input: `<html><meta name="go-source" content="gopkg.in/natefinch/lumberjack.v2 _ https://github.com/natefinch/lumberjack/tree/v2.1{/dir} https://github.com/natefinch/lumberjack/blob/v2.1{/dir}/{file}#L{line}"></html>`,
			expected: GoSource{
				ImportPrefix: "gopkg.in/natefinch/lumberjack.v2",
				Home:         "_",
				Directory:    "https://github.com/natefinch/lumberjack/tree/v2.1{/dir}",
				File:         "https://github.com/natefinch/lumberjack/blob/v2.1{/dir}/{file}#L{line}",
			},
			expectedOK: true,
		},
	}

	for i, test := range tests {
		gs, ok := parseGoSource(test.input)
		if ok != test.expectedOK {
			t.Errorf("test %d failed: parse error", i)
		} else if gs != test.expected {
			t.Errorf("test %d failed: expected %+v but got %+v",
				i, test.expected, gs)
		}
	}
}
