package auth

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

// NewUUIDv7 generates a UUIDv7 (time-sortable) as a string.
func NewUUIDv7() string {
	now := time.Now()
	ms := uint64(now.UnixMilli())

	var b [16]byte

	// Timestamp: 48 bits in bytes 0-5.
	binary.BigEndian.PutUint16(b[0:2], uint16(ms>>32))
	binary.BigEndian.PutUint32(b[2:6], uint32(ms))

	// Random: fill bytes 6-15.
	//
	// The error is deliberately not checked, and a guard for it would be a
	// branch that cannot be taken: crypto/rand.Read never returns one. It
	// always fills b, and crashes the program irrecoverably if the system
	// source fails, so there is no path here that yields a low-entropy UUID.
	// Raised by an outside review on 2026-09-16 and answered in docs/adr/113.
	rand.Read(b[6:])

	// Version 7: set bits in byte 6.
	b[6] = (b[6] & 0x0F) | 0x70

	// Variant 10xx: set bits in byte 8.
	b[8] = (b[8] & 0x3F) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(b[0:4]),
		binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]),
		binary.BigEndian.Uint16(b[8:10]),
		b[10:16],
	)
}
