package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

func main() {
	err := errors.New("something failed")
	userID := 123
	log.Error().
		Err(err).
		Str("context", "db_query").
		Int("userID", userID).
		Bool("dbError", true).
		Msg("DB query failed for user")

	i8 := int8(8)
	i16 := int16(16)
	i32 := int32(32)
	i64 := int64(64)
	log.Info().
		Int8("int8", i8).
		Int16("int16", i16).
		Int32("int32", i32).
		Int64("int64", i64).
		Msg("integer logging")

	u8 := uint8(8)
	u16 := uint16(16)
	u32 := uint32(32)
	u64 := uint64(64)
	log.Info().
		Uint8("uint8", u8).
		Uint16("uint16", u16).
		Uint32("uint32", u32).
		Uint64("uint64", u64).
		Msg("unsinged integer logging")

	f32 := float32(32.0)
	f64 := float64(64.0)
	log.Info().
		Float32("float32", f32).
		Float64("float64", f64).
		Msg("unsinged integer logging")

	t := time.Now()
	dur := 10 * time.Second
	log.Warn().
		Time("time", t).
		Dur("duration", dur).
		Msg("time logging")

	// Zerolog Debug call
	log.Debug().Msg("Debug event")

	// A call that should be ignored
	fmt.Println("This is a regular function call")
}
