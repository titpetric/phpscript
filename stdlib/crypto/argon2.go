package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// The work factors php defaults to, reported through the three
// PASSWORD_ARGON2_DEFAULT_* constants. They are RFC 9106's second recommended
// parameter set and cost more wall clock than bcrypt at cost 12; a deployment
// that wants the trade the other way lowers memory_cost and leaves time_cost
// where it is, which is argued in docs/reference/extensions/hashing.md.
const (
	argon2DefaultMemory = 65536
	argon2DefaultTime   = 4
	argon2DefaultLanes  = 1
)

// The shape of the encoded hash, which is libargon2's and therefore php's: a
// 16-byte salt and a 32-byte tag, in the PHC string form with version 0x13.
// Writing the same shape is what makes a hash written here verify under php
// and the other way around.
const (
	argon2SaltLen = 16
	argon2KeyLen  = 32
	argon2Version = 19
)

// argon2MaxLanes is a limit of this implementation rather than of argon2.
// x/crypto takes the parallelism as a uint8, and php accepts larger values, so
// a threads option above this is refused instead of silently truncated.
const argon2MaxLanes = 255

// argon2b64 is the PHC string format's encoding: standard base64, unpadded.
var argon2b64 = base64.RawStdEncoding

// argon2Hash derives a fresh hash under p and encodes it with the salt it
// generated, because a verify has no other way to recover the salt.
func argon2Hash(password string, p params) (string, error) {
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	return argon2Encode(p, salt, argon2Key(p, []byte(password), salt, argon2KeyLen)), nil
}

// argon2Verify recomputes the tag from the parameters and salt the stored hash
// carries. A hash it cannot read is false rather than an error, which is the
// answer password_verify gives for every malformed input.
func argon2Verify(password, hash string) bool {
	p, salt, want, err := argon2Decode(hash)
	if err != nil {
		return false
	}
	got := argon2Key(p, []byte(password), salt, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// argon2Key runs the variant p names. argon2i is data-independent and argon2id
// runs one data-independent pass before the data-dependent ones; both are
// reachable because php registers both, and argon2id is the one to choose.
func argon2Key(p params, password, salt []byte, keyLen uint32) []byte {
	if p.algo == algoArgon2i {
		return argon2.Key(password, salt, p.time, p.memory, p.lanes, keyLen)
	}
	return argon2.IDKey(password, salt, p.time, p.memory, p.lanes, keyLen)
}

// argon2Encode writes the PHC string form.
func argon2Encode(p params, salt, sum []byte) string {
	var b strings.Builder
	b.WriteString("$")
	b.WriteString(p.algo)
	b.WriteString("$v=")
	b.WriteString(strconv.Itoa(argon2Version))
	b.WriteString("$m=")
	b.WriteString(strconv.FormatUint(uint64(p.memory), 10))
	b.WriteString(",t=")
	b.WriteString(strconv.FormatUint(uint64(p.time), 10))
	b.WriteString(",p=")
	b.WriteString(strconv.FormatUint(uint64(p.lanes), 10))
	b.WriteString("$")
	b.WriteString(argon2b64.EncodeToString(salt))
	b.WriteString("$")
	b.WriteString(argon2b64.EncodeToString(sum))
	return b.String()
}

// argon2Decode reads the PHC string form back into the parameters, the salt
// and the stored tag. The version field is required: a string without it is
// argon2 1.0, which derives a different tag, so reading one as 1.3 would
// report a wrong answer rather than no answer.
func argon2Decode(hash string) (params, []byte, []byte, error) {
	fields := strings.Split(hash, "$")
	if len(fields) != 6 || fields[0] != "" {
		return params{}, nil, nil, errors.New("not an argon2 hash")
	}

	p := params{algo: fields[1]}
	if p.algo != algoArgon2i && p.algo != algoArgon2id {
		return params{}, nil, nil, fmt.Errorf("unknown argon2 variant %q", p.algo)
	}
	if fields[2] != "v="+strconv.Itoa(argon2Version) {
		return params{}, nil, nil, fmt.Errorf("unsupported argon2 version %q", fields[2])
	}

	for _, field := range strings.Split(fields[3], ",") {
		name, value, ok := strings.Cut(field, "=")
		if !ok {
			return params{}, nil, nil, fmt.Errorf("malformed argon2 parameter %q", field)
		}
		number, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return params{}, nil, nil, fmt.Errorf("malformed argon2 parameter %q", field)
		}
		switch name {
		case "m":
			p.memory = uint32(number)
		case "t":
			p.time = uint32(number)
		case "p":
			if number > argon2MaxLanes {
				return params{}, nil, nil, fmt.Errorf("argon2 parallelism %d is above the %d this build supports", number, argon2MaxLanes)
			}
			p.lanes = uint8(number)
		default:
			return params{}, nil, nil, fmt.Errorf("unknown argon2 parameter %q", name)
		}
	}
	if err := p.validate(); err != nil {
		return params{}, nil, nil, err
	}

	salt, err := argon2b64.DecodeString(fields[4])
	if err != nil {
		return params{}, nil, nil, fmt.Errorf("malformed argon2 salt: %w", err)
	}
	sum, err := argon2b64.DecodeString(fields[5])
	if err != nil {
		return params{}, nil, nil, fmt.Errorf("malformed argon2 hash: %w", err)
	}
	if len(sum) == 0 {
		return params{}, nil, nil, errors.New("argon2 hash is empty")
	}
	return p, salt, sum, nil
}

// isArgon2 reports whether a stored hash is one of the two variants, which is
// how password_verify picks a derivation without being told.
func isArgon2(hash string) bool {
	return strings.HasPrefix(hash, "$"+algoArgon2i+"$") || strings.HasPrefix(hash, "$"+algoArgon2id+"$")
}
