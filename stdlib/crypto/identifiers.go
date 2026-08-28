package crypto

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/titpetric/phpscript/runner"
)

// RegisterIdentifiers installs the id generators. Neither is a PHP name:
// PHP's own library mints no unique ids, so ports reach for composer packages
// (ramsey/uuid, symfony/ulid) whose value is exactly the entropy source this
// package already owns. Both are implemented here rather than through a Go
// dependency, because each is a format over crypto/rand and nothing more.
//
// The two share one layout — a 48-bit millisecond timestamp followed by
// randomness — so both sort by creation time. ulid() renders it in Crockford
// base32; uuid() renders it as a version 7 UUID, the hexadecimal spelling of
// the same idea.
func RegisterIdentifiers(rt *runner.Runtime) {
	// ulid returns a 26-character ULID: a millisecond timestamp and 80 random bits in Crockford base32, so ids sort by creation time. Ids from the same millisecond sort in no particular order.
	rt.RegisterFunc("ulid", func() (string, error) {
		b, err := timestampedID()
		if err != nil {
			return "", fmt.Errorf("ulid(): %w", err)
		}
		return encodeCrockford(b), nil
	})

	// uuid returns a UUIDv7 as 36 lowercase characters in the 8-4-4-4-12 form: a millisecond timestamp and random bits, so ids sort by creation time like a ulid.
	rt.RegisterFunc("uuid", func() (string, error) {
		b, err := timestampedID()
		if err != nil {
			return "", fmt.Errorf("uuid(): %w", err)
		}
		b[6] = (b[6] & 0x0f) | 0x70 // version 7
		b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
		var out [36]byte
		hex.Encode(out[:8], b[:4])
		out[8] = '-'
		hex.Encode(out[9:13], b[4:6])
		out[13] = '-'
		hex.Encode(out[14:18], b[6:8])
		out[18] = '-'
		hex.Encode(out[19:23], b[8:10])
		out[23] = '-'
		hex.Encode(out[24:], b[10:])
		return string(out[:]), nil
	})
}

// timestampedID builds the 16 bytes both generators format: the current
// millisecond in the top 48 bits, crypto/rand in the remaining 80.
func timestampedID() ([16]byte, error) {
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], uint64(time.Now().UnixMilli())<<16)
	if _, err := rand.Read(b[6:]); err != nil {
		return b, err
	}
	return b, nil
}

// encodeCrockford renders 16 bytes as the 26-character ULID text form:
// Crockford base32, most significant bits first, with the 128-bit value
// padded to 130 bits by two leading zero bits.
func encodeCrockford(b [16]byte) string {
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	hi := binary.BigEndian.Uint64(b[:8])
	lo := binary.BigEndian.Uint64(b[8:])
	var out [26]byte
	for i := range out {
		shift := uint(5 * (25 - i))
		var v uint64
		switch {
		case shift >= 64:
			v = hi >> (shift - 64)
		case shift == 0:
			v = lo
		default:
			v = lo>>shift | hi<<(64-shift)
		}
		out[i] = alphabet[v&31]
	}
	return string(out[:])
}
