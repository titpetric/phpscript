package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// argon2DefaultMemory, argon2DefaultTime and argon2DefaultLanes hold php's RFC 9106 default work factors.
const (
	argon2DefaultMemory = 65536
	argon2DefaultTime   = 4
	argon2DefaultLanes  = 1
)

// argon2SaltLen and argon2KeyLen hold the 16-byte salt and 32-byte tag php's PHC form uses.
const (
	argon2SaltLen = 16
	argon2KeyLen  = 32
	argon2Version = 19
)

// argon2MaxLanes caps parallelism at this build's uint8 limit, refusing a larger value.
const argon2MaxLanes = 255

// argon2b64 is the PHC string format's encoding: standard base64, unpadded.
var argon2b64 = base64.RawStdEncoding

// argon2Hash derives a fresh hash under p, encoding the generated salt into the result.
func argon2Hash(password string, p params) (string, error) {
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	return argon2Encode(salt, argon2Key(p, []byte(password), salt, argon2KeyLen), p), nil
}

// argon2Verify recomputes the tag from the stored hash's parameters and salt, false when unreadable.
func argon2Verify(password, hash string) bool {
	p, salt, want, err := argon2Decode(hash)
	if err != nil {
		return false
	}
	got := argon2Key(p, []byte(password), salt, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// argon2Encode writes the PHC string form.
func argon2Encode(salt, sum []byte, p params) string {
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

// argon2Decode reads the PHC string into parameters, salt and tag, requiring the version field.
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
	if err := p.check(); err != nil {
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

// isArgon2 reports whether a stored hash names an argon2i or argon2id variant.
func isArgon2(hash string) bool {
	return strings.HasPrefix(hash, "$"+algoArgon2i+"$") || strings.HasPrefix(hash, "$"+algoArgon2id+"$")
}
