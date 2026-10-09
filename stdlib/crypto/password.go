// Package crypto holds the password hashing and the random source a script
// cannot write for itself.
//
// PHP's password_hash() is one of the few standard functions with no
// implementation in the language: it needs a key derivation function and a
// CSPRNG, and phpscript exposes neither. Everything else the family does
// (parsing options, choosing an algorithm) is arithmetic a script could do, so
// the binding is narrow: the password functions here, and the CSPRNG pair in
// random.go, which returns the same entropy to scripts directly.
package crypto

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// The PHP algorithm identifiers. PASSWORD_DEFAULT is bcrypt in PHP 8, so it is
// the same value as PASSWORD_BCRYPT and a script that passes either gets the
// same hash; argon2id is the stronger choice per millisecond spent and is not
// the default here for the same reason it is not the default there, which is
// that it changes the memory footprint of a login.
const (
	algoBcrypt   = "2y"
	algoArgon2i  = "argon2i"
	algoArgon2id = "argon2id"

	// defaultCost is PHP 8.5's PASSWORD_BCRYPT_DEFAULT_COST, not x/crypto's
	// bcrypt.DefaultCost, which is still 10. The number is reported to
	// scripts through the constant and is recorded inside every hash, so a
	// difference here is a difference a script can see; matching PHP is what
	// lets the .phpt fixture be checked against the php binary.
	defaultCost = 12
)

// phpPrefix is what PHP's crypt() writes and x/crypto refuses; goPrefix is
// what x/crypto writes and PHP accepts.
//
// The two are the same algorithm. The `y` revision was PHP's marker for the
// 2011 fix to the sign-extension bug, which Go's implementation never had, and
// x/crypto rejects any minor version outside the set it reads. Rewriting the
// prefix on the way in and out makes a hash written here verify under php, and
// a hash written by php verify here.
const (
	phpPrefix = "$2y$"
	goPrefix  = "$2a$"
)

// params is the algorithm and the work factors one hash was written at. One
// type covers both algorithms because password_hash takes the options of
// whichever algorithm it was handed; only the fields that algorithm reads are
// set, and validate() is what decides which those are.
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

// validate refuses a parameter set the algorithm cannot run. The argon2 bounds
// are libargon2's, so php refuses the same values with the same meaning: one
// lane minimum, one pass minimum, and at least eight KiB per lane.
func (p params) validate() error {
	if p.algo == algoBcrypt {
		if p.cost < bcrypt.MinCost || p.cost > bcrypt.MaxCost {
			return fmt.Errorf("cost must be between %d and %d, got %d", bcrypt.MinCost, bcrypt.MaxCost, p.cost)
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

	// password_verify reports whether $password produced $hash. The algorithm
	// and the work factors are read out of $hash. An empty or malformed $hash
	// returns false; it raises no error.
	rt.RegisterFunc("password_verify", passwordVerify)

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
			// Not a hash this build can read, so anything else is an upgrade.
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

// passwordVerify answers password_verify. The stored hash names the algorithm
// and the work factors, so a login costs whatever the hash was written at and
// nothing here can lower that.
//
// What it can do is not pay a derivation for a value no derivation could match.
// A string that is neither argon2 nor long enough to be bcrypt is false in the
// time it takes to read its first byte, which is also what php answers in: an
// empty hash is 31 ns there. Spending a derivation on it instead would turn a
// request carrying a wrong-shaped cookie into a fifth of a second of one core,
// which is a request-rate limit an unauthenticated caller sets.
func passwordVerify(password, hash string) bool {
	switch {
	case isArgon2(hash):
		return argon2Verify(password, hash)
	case isBcrypt(hash):
		return bcrypt.CompareHashAndPassword([]byte(goForm(hash)), []byte(password)) == nil
	default:
		return false
	}
}

// algoName maps an identifier to the name password_get_info reports, which is
// the identifier itself for argon2 and a different word for bcrypt.
func algoName(algo string) string {
	if algo == algoBcrypt {
		return "bcrypt"
	}
	return algo
}

// derive runs the algorithm p names. The bcrypt branch rewrites the revision
// marker on the way out; the argon2 branch writes the PHC string form, which
// both implementations already agree on.
func derive(password string, p params) (string, error) {
	if p.algo != algoBcrypt {
		return argon2Hash(password, p)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), p.cost)
	if err != nil {
		return "", err
	}
	return phpPrefix + strings.TrimPrefix(string(hash), goPrefix), nil
}

// paramsOf reads back the parameters a stored hash was written at.
// password_get_info and password_needs_rehash both answer from it.
func paramsOf(hash string) (params, bool) {
	if isArgon2(hash) {
		p, _, _, err := argon2Decode(hash)
		return p, err == nil
	}
	if !isBcrypt(hash) {
		return params{}, false
	}
	cost, err := bcrypt.Cost([]byte(goForm(hash)))
	if err != nil {
		return params{}, false
	}
	return params{algo: algoBcrypt, cost: cost}, true
}

// isBcrypt reports whether a stored value is long enough and shaped right to be
// a bcrypt hash. It is the two conditions x/crypto rejects a string on before it
// parses anything - 59 bytes and a leading "$" - asked here so that a value
// which is neither argon2 nor bcrypt is answered without copying it to a []byte.
//
// It decides nothing else. The revision marker, the cost digits and the base64
// are x/crypto's to read, so a hash this accepts can still be refused, and the
// set of hashes that verify is the same as if password_verify called it blind.
func isBcrypt(hash string) bool {
	return len(hash) >= 59 && hash[0] == '$'
}

// goForm rewrites a PHP-written hash into the revision x/crypto accepts.
func goForm(hash string) string {
	if strings.HasPrefix(hash, phpPrefix) {
		return goPrefix + strings.TrimPrefix(hash, phpPrefix)
	}
	return hash
}

// paramsFrom reads the ($algo, $options) tail both hashing functions take. PHP
// allows the algorithm to be omitted in neither position, but it does allow
// either to be null, and a script that passes only options is the common case
// once the algorithm is whatever PASSWORD_DEFAULT resolves to.
//
// The algorithm is resolved before the options are read, because the defaults
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

	if err := p.validate(); err != nil {
		return params{}, err
	}
	return p, nil
}

// algoFrom picks the algorithm out of the tail. A bare number is PHP 7's
// spelling of the identifiers, where 1 was bcrypt, 2 argon2i and 3 argon2id;
// PHP 8 refuses an integer outright, and accepting it is what keeps a script
// written against the older constants from silently storing the wrong
// algorithm.
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
// is what PASSWORD_DEFAULT was in that version.
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
