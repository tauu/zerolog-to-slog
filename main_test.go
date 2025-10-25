package main

import (
	"bytes"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/dstutil"
)

func TestZerologToSlogConversion(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "Simple Info message",
			input: `package main

import "github.com/rs/zerolog/log"

func main() {
	log.Info().Msg("hello world")
}`,
			expected: `package main

import "log/slog"

func main() {
	slog.LogAttrs(ctx, slog.LevelInfo, "hello world")
}`,
		},
		{
			name: "Info message with string field",
			input: `package main

import "github.com/rs/zerolog/log"

func main() {
	log.Info().Str("key", "value").Msg("hello world")
}`,
			expected: `package main

import "log/slog"

func main() {
	slog.LogAttrs(ctx, slog.LevelInfo, "hello world", slog.String("key", "value"))
}`,
		},
		{
			name: "Debug message with int and bool fields",
			input: `package main

import "github.com/rs/zerolog/log"

func main() {
	log.Debug().Int("num", 42).Bool("active", true).Msg("debugging")
}`,
			expected: `package main

import "log/slog"

func main() {
	slog.LogAttrs(ctx, slog.LevelDebug, "debugging", slog.Int("num", 42), slog.Bool("active", true))
}`,
		},
		{
			name: "Error with an error field",
			input: `package main

import (
	"errors"
	"github.com/rs/zerolog/log"
)

func main() {
	err := errors.New("an error")
	log.Error().Err(err).Msg("something went wrong")
}`,
			expected: `package main

import (
	"errors"
	"log/slog"
)

func main() {
	err := errors.New("an error")
	slog.LogAttrs(ctx, slog.LevelError, "something went wrong", slog.Any("err", err))
}`,
		},
		{
			name: "Msgf formatting",
			input: `package main

import "github.com/rs/zerolog/log"

func main() {
	log.Warn().Msgf("user %s logged in", "testuser")
}`,
			expected: `package main

import "log/slog"

func main() {
	slog.LogAttrs(ctx, slog.LevelWarn, "user %s logged in")
}`,
		},
		{
			name: "Send with no message",
			input: `package main

import "github.com/rs/zerolog/log"

func main() {
	log.Info().Str("key", "value").Send()
}`,
			expected: `package main

import "log/slog"

func main() {
	slog.LogAttrs(ctx, slog.LevelInfo, "zerolog event", slog.String("key", "value"))
}`,
		},
		{
			name: "Fatal level",
			input: `package main

import "github.com/rs/zerolog/log"

func main() {
	log.Fatal().Msg("critical error")
}`,
			expected: `package main

import "log/slog"

func main() {
	slog.LogAttrs(ctx, slog.LevelError, "critical error")
}`,
		},
		{
			name: "Panic level",
			input: `package main

import "github.com/rs/zerolog/log"

func main() {
	log.Panic().Msg("unrecoverable error")
}`,
			expected: `package main

import "log/slog"

func main() {
	slog.LogAttrs(ctx, slog.LevelError, "unrecoverable error")
}`,
		},
		{
			name: "Multiple data types",
			input: `package main

import (
	"time"
	"github.com/rs/zerolog/log"
)

func main() {
	log.Info().
		Str("string", "a").
		Int("int", 1).
		Int8("int8", 2).
		Int16("int16", 3).
		Int32("int32", 4).
		Int64("int64", 5).
		Uint("uint", 6).
		Uint8("uint8", 7).
		Uint16("uint16", 8).
		Uint32("uint32", 9).
		Uint64("uint64", 10).
		Float32("float32", 11.1).
		Float64("float64", 12.2).
		Bool("bool", true).
		Time("time", time.Now()).
		Dur("duration", time.Second).
		Msg("complex message")
}`,
			expected: `package main

import (
	"log/slog"
	"time"
)

func main() {
	slog.LogAttrs(ctx, slog.LevelInfo, "complex message", slog.String("string", "a"), slog.Int("int", 1), slog.Int("int8", int(2)), slog.Int("int16", int(3)), slog.Int("int32", int(4)), slog.Int64("int64", 5), slog.Uint64("uint", uint64(6)), slog.Uint64("uint8", uint64(7)), slog.Uint64("uint16", uint64(8)), slog.Uint64("uint32", uint64(9)), slog.Uint64("uint64", 10), slog.Float64("float32", float64(11.1)), slog.Float64("float64", 12.2), slog.Bool("bool", true), slog.Time("time", time.Now()), slog.Duration("duration", time.Second))
}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			node, err := decorator.ParseFile(fset, "", tc.input, parser.ParseComments)
			if err != nil {
				t.Fatalf("Failed to parse input: %v", err)
			}

			v := &RewriteVisitor{fileSet: fset, Replacements: make(map[dst.Node]dst.Node)}
			dst.Walk(v, node)

			// Apply replacements
			dstutil.Apply(node, nil, func(c *dstutil.Cursor) bool {
				if replacement, ok := v.Replacements[c.Node()]; ok {
					c.Replace(replacement)
				}
				return true
			})

			var buf bytes.Buffer
			if err := decorator.Fprint(&buf, node); err != nil {
				t.Fatalf("Failed to print output: %v", err)
			}

			if got := buf.String(); strings.Trim(got, "\n") != tc.expected {
				t.Errorf(`Unexpected output.
Got:
%s
Expected:
%s`, got, tc.expected)
			}
		})
	}
}
