// Package crypto holds the password hashing and the random source a script
// cannot write for itself.
//
// PHP's password_hash() is one of the few standard functions with no
// implementation in the language: it needs a key derivation function and a
// CSPRNG, and phpscript exposes neither. Everything else the family does
// (parsing options, choosing an algorithm) is arithmetic a script could do and
// the binding is narrow: the password functions here, and the CSPRNG pair in
// random.go, which returns the same entropy to scripts directly.
package crypto

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// The PHP algorithm identifiers. PASSWORD_DEFAULT is bcrypt in PHP 8 and it
// carries the same value as PASSWORD_BCRYPT and a script that passes either
// gets the same hash; argon2id is the stronger choice per millisecond spent and
// is not the default here for the same reason php does not default to it: it
// changes the memory footprint of a login.
const (
	algoBcrypt   = "2y"
	algoArgon2i  = "argon2i"
	algoArgon2id = "argon2id"

	// defaultCost is PHP 8.5's PASSWORD_BCRYPT_DEFAULT_COST, not x/crypto's
	// bcrypt.DefaultCost, which is still 10. The constant reports the number to
	// scripts and every hash records it and a difference here is a difference a
	// script can see; matching PHP is what lets the .phpt fixture run against
	// the php binary.
	defaultCost = 12
)

// phpPrefix is the revision php's password_hash writes; goPrefix is the one
// x/crypto's GenerateFromPassword writes. Both name the same derivation.
//
// Reading needs no rewrite: x/crypto refuses only a major version above '2'
// and carries the minor through the comparison and a stored `$2y$`, `$2a$` or
// `$2b$` verifies as it stands. Writing does need one, because the prefix is
// script-visible: derive swaps goPrefix for phpPrefix on the way out, and a
// hash written here reads byte for byte like php's.
const (
	phpPrefix = "$2y$"
	goPrefix  = "$2a$"
)

// params is the algorithm and the work factors of one hash. One type covers
// both algorithms because password_hash takes the options of whichever
// algorithm the call names; defaultParams and readOptions fill only the fields
// that algorithm reads, and the bounds check tests the same ones.
type params struct {
	algo   string
	cost   int    // bcrypt, log2 of the key schedule iterations
	memory uint32 // argon2, KiB
	time   uint32 // argon2, passes over that memory
	lanes  uint8  // argon2, parallelism
}

// defaultParams is what an omitted $options means, per algorithm.
func defaultParams(algo string) params {
	if algo == algoBcrypt {
		return params{algo: algo, cost: defaultCost}
	}
	return params{
		algo:   algo,
		memory: argon2DefaultMemory,
		time:   argon2DefaultTime,
		lanes:  argon2DefaultLanes,
	}
}

// check refuses a parameter set the algorithm cannot run. The argon2 bounds
// are libargon2's and php refuses the same values with the same meaning: at
// least one lane, at least one pass, and at least eight KiB per lane.
func (p params) check() error {
	if p.algo == algoBcrypt {
		if p.cost < bcryptMinCost || p.cost > bcryptMaxCost {
			return fmt.Errorf("cost must be between %d and %d, got %d", bcryptMinCost, bcryptMaxCost, p.cost)
		}
		return nil
	}
	switch {
	case p.lanes < 1:
		return errors.New("invalid number of threads")
	case p.time < 1:
		return errors.New("time cost is outside of allowed time range")
	case p.memory < 8:
		return errors.New("memory cost is outside of allowed memory range")
	case p.memory < 8*uint32(p.lanes):
		return fmt.Errorf("memory cost %d is too small for %d threads", p.memory, p.lanes)
	}
	return nil
}

// options renders the parameters the way password_get_info reports them, which
// is one key per knob the algorithm has.
func (p params) options() *model.Array {
	out := model.NewArray()
	if p.algo == algoBcrypt {
		out.Set("cost", int64(p.cost))
		return out
	}
	out.Set("memory_cost", int64(p.memory))
	out.Set("time_cost", int64(p.time))
	out.Set("threads", int64(p.lanes))
	return out
}

// Register installs the password hashing functions and their constants on rt.
func Register(rt *runner.Runtime) {
	rt.SetConst("PASSWORD_BCRYPT", algoBcrypt)
	rt.SetConst("PASSWORD_DEFAULT", algoBcrypt)
	rt.SetConst("PASSWORD_BCRYPT_DEFAULT_COST", int64(defaultCost))
	rt.SetConst("PASSWORD_ARGON2I", algoArgon2i)
	rt.SetConst("PASSWORD_ARGON2ID", algoArgon2id)
	rt.SetConst("PASSWORD_ARGON2_DEFAULT_MEMORY_COST", int64(argon2DefaultMemory))
	rt.SetConst("PASSWORD_ARGON2_DEFAULT_TIME_COST", int64(argon2DefaultTime))
	rt.SetConst("PASSWORD_ARGON2_DEFAULT_THREADS", int64(argon2DefaultLanes))
	rt.SetConst("PASSWORD_ARGON2_PROVIDER", "standard")

	// password_hash returns a salted hash of $password under $algo, which is
	// PASSWORD_BCRYPT, PASSWORD_ARGON2ID or PASSWORD_ARGON2I. $options takes
	// "cost" for bcrypt and "memory_cost", "time_cost" and "threads" for
	// argon2. The salt comes from the system CSPRNG.
	rt.RegisterFunc("password_hash", func(password string, opts ...any) (string, error) {
		p, err := paramsFrom(opts)
		if err != nil {
			return "", fmt.Errorf("password_hash(): %w", err)
		}
		hash, err := derive(password, p)
		if err != nil {
			return "", fmt.Errorf("password_hash(): %w", err)
		}
		return hash, nil
	})

	// password_verify reports whether $password produced $hash. It reads the
	// algorithm and the work factors out of $hash. An empty or malformed $hash
	// returns false; it raises no error.
	rt.RegisterFunc("password_verify", PasswordVerify)

	// password_needs_rehash reports whether $hash records a different algorithm
	// or different work factors than $algo and $options name. A login that
	// already holds the plaintext uses it to re-store the hash.
	rt.RegisterFunc("password_needs_rehash", func(hash string, opts ...any) (bool, error) {
		want, err := paramsFrom(opts)
		if err != nil {
			return false, fmt.Errorf("password_needs_rehash(): %w", err)
		}
		got, ok := paramsOf(hash)
		if !ok {
			// Not a hash this build can read and anything else is an upgrade.
			return true, nil
		}
		return got != want, nil
	})

	// password_get_info returns the algorithm and the work factors $hash
	// records. An unrecognised hash reports a null algo and the algoName
	// "unknown", as PHP's does.
	rt.RegisterFunc("password_get_info", func(hash string) *model.Array {
		info := model.NewArray()

		p, ok := paramsOf(hash)
		if !ok {
			info.Set("algo", nil)
			info.Set("algoName", "unknown")
			info.Set("options", model.NewArray())
			return info
		}

		info.Set("algo", p.algo)
		info.Set("algoName", algoName(p.algo))
		info.Set("options", p.options())
		return info
	})

	// password_algos returns the algorithm identifiers password_hash() accepts,
	// in php's order: bcrypt, then the two argon2 variants.
	rt.RegisterFunc("password_algos", func() []any {
		return []any{algoBcrypt, algoArgon2i, algoArgon2id}
	})
}

// PasswordVerify answers password_verify, and is what stdlib/auth reads a
// stored password file line with: a user database holds what password_hash
// wrote and the dispatch has one home.
//
// The stored hash names the algorithm and the work factors and a login costs
// what that hash carries. A value no derivation could match reads false in
// the time its first byte takes, as in php, which keeps a wrong-shaped cookie
// from costing a core.
func PasswordVerify(password, hash string) bool {
	switch {
	case isArgon2(hash):
		return argon2Verify(password, hash)
	case isBcrypt(hash):
		return bcryptVerify(password, hash)
	default:
		return false
	}
}

// IsPasswordHash reports whether PasswordVerify can read a stored value: one
// of the formats it dispatches on, with the parameters inside it readable. A
// value this refuses matches no password and a user database asks it when it
// reads a file and stops there, ahead of the login that would fail.
//
// It runs the parse and no prefix test. The shape checks the dispatch uses are
// loose by design and that a hash php wrote and this build cannot read answers
// false from password_verify; here a loose answer would accept a damaged line.
func IsPasswordHash(stored string) bool {
	if isArgon2(stored) {
		_, _, _, err := argon2Decode(stored)
		return err == nil
	}
	if !bcryptEncoded(stored) {
		return false
	}
	_, err := bcryptCost(stored)
	return err == nil
}

// algoName maps an identifier to the name password_get_info reports, which is
// the identifier itself for argon2 and a different word for bcrypt.
func algoName(algo string) string {
	if algo == algoBcrypt {
		return "bcrypt"
	}
	return algo
}

// paramsOf reads back the parameters a stored hash carries.
// password_get_info and password_needs_rehash both answer from it.
func paramsOf(hash string) (params, bool) {
	if isArgon2(hash) {
		p, _, _, err := argon2Decode(hash)
		return p, err == nil
	}
	if !isBcrypt(hash) {
		return params{}, false
	}
	cost, err := bcryptCost(hash)
	if err != nil {
		return params{}, false
	}
	return params{algo: algoBcrypt, cost: cost}, true
}

// isBcrypt reports whether a stored value has the length and the revision
// marker of a bcrypt hash. The length is what x/crypto rejects a string on
// before it parses anything, run here first so a value that is neither argon2
// nor bcrypt answers false without a copy to a []byte.
//
// The revision marker is the one place this is stricter than x/crypto, which
// parses the 1997 "$2$" spelling php refuses; bcryptRevision names the four
// markers php reads. The cost digits and the base64 stay x/crypto's to read,
// and it can still refuse a hash this accepts.
func isBcrypt(hash string) bool {
	return len(hash) >= 59 && bcryptRevision(hash)
}

// paramsFrom reads the ($algo, $options) tail both hashing functions take. PHP
// requires the algorithm in both positions, though it does allow either to be
// null, and a script that passes only options is the common case once the
// algorithm is whatever PASSWORD_DEFAULT resolves to.
//
// It resolves the algorithm before reading the options, because the defaults
// an absent key falls back to are the algorithm's own.
func paramsFrom(opts []any) (params, error) {
	algo, err := algoFrom(opts)
	if err != nil {
		return params{}, err
	}

	p := defaultParams(algo)
	for _, opt := range opts {
		options, ok := opt.(*model.Array)
		if !ok {
			continue
		}
		if err := p.readOptions(options); err != nil {
			return params{}, err
		}
	}

	if err := p.check(); err != nil {
		return params{}, err
	}
	return p, nil
}

// algoFrom picks the algorithm out of the tail. A bare number is PHP 7's
// spelling of the identifiers: 1 is bcrypt, 2 argon2i and 3 argon2id.
// PHP 8 refuses an integer outright; accepting one here keeps a script written
// against the older constants from silently storing the wrong algorithm.
func algoFrom(opts []any) (string, error) {
	algo := algoBcrypt
	for _, opt := range opts {
		switch value := opt.(type) {
		case nil, *model.Array:
			continue
		case string:
			switch value {
			case algoBcrypt, "bcrypt":
				algo = algoBcrypt
			case algoArgon2i, algoArgon2id:
				algo = value
			default:
				return "", fmt.Errorf("unsupported algorithm %q, expected one of 2y, argon2i, argon2id", value)
			}
		case int64:
			algo = legacyAlgo(value)
		case int:
			algo = legacyAlgo(int64(value))
		case float64:
			algo = legacyAlgo(int64(value))
		default:
			return "", fmt.Errorf("argument must be an algorithm or an options array, got %T", opt)
		}
	}
	return algo, nil
}

// legacyAlgo maps PHP 7's integer identifiers. Anything else is bcrypt, which
// is what PASSWORD_DEFAULT names in that version.
func legacyAlgo(value int64) string {
	switch value {
	case 2:
		return algoArgon2i
	case 3:
		return algoArgon2id
	default:
		return algoBcrypt
	}
}

// readOptions applies the keys the algorithm reads and ignores the rest, which
// is what php does with an option belonging to another algorithm.
func (p *params) readOptions(options *model.Array) error {
	if p.algo == algoBcrypt {
		cost, err := intOption(options, "cost", int64(p.cost))
		if err != nil {
			return err
		}
		p.cost = int(cost)
		return nil
	}

	memory, err := intOption(options, "memory_cost", int64(p.memory))
	if err != nil {
		return err
	}
	time, err := intOption(options, "time_cost", int64(p.time))
	if err != nil {
		return err
	}
	lanes, err := intOption(options, "threads", int64(p.lanes))
	if err != nil {
		return err
	}
	if memory < 0 || memory > math32Max || time < 0 || time > math32Max {
		return errors.New("memory cost is outside of allowed memory range")
	}
	if lanes < 0 || lanes > argon2MaxLanes {
		return fmt.Errorf("threads must be between 1 and %d, got %d", argon2MaxLanes, lanes)
	}
	p.memory, p.time, p.lanes = uint32(memory), uint32(time), uint8(lanes)
	return nil
}

// math32Max is the widest value the encoded parameters can carry, which is
// also the widest x/crypto takes.
const math32Max = 1<<32 - 1

// intOption reads one option key, which arrives as whatever the script wrote
// it as, and answers fallback when the key is absent.
func intOption(options *model.Array, name string, fallback int64) (int64, error) {
	value, ok := options.Get(name)
	if !ok {
		return fallback, nil
	}

	switch number := value.(type) {
	case int64:
		return number, nil
	case int:
		return int64(number), nil
	case float64:
		return int64(number), nil
	case string:
		parsed, err := strconv.ParseInt(number, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%s %q is not a number", name, number)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("%s must be a number, got %T", name, value)
	}
}
