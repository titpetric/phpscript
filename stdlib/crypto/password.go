// Package crypto holds the password hashing a script cannot write for itself.
//
// PHP's password_hash() is one of the few standard functions with no
// implementation in the language: it needs a key derivation function and a
// CSPRNG, and phpscript exposes neither. Everything else the family does
// (parsing options, choosing an algorithm) is arithmetic a script could do, so
// the binding is deliberately three functions wide and no wider.
package crypto

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// The PHP algorithm identifiers. PASSWORD_DEFAULT is bcrypt in PHP 8, and
// bcrypt is the only algorithm phpscript implements, so the two are the same
// value and a script that passes either gets the same hash.
const (
	algoBcrypt = "2y"

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
// x/crypto rejects any minor version it does not know. Rewriting the prefix on
// the way in and out is what makes a hash written here verify there and the
// other way around, which is the whole point of implementing the PHP function
// rather than exposing bcrypt under its own name.
const (
	phpPrefix = "$2y$"
	goPrefix  = "$2a$"
)

// dummyHash is compared against when there is no hash to check, so a caller
// that looks up a user first spends the same time on a name that does not
// exist as on one that does. Without it the response time answers "is this a
// real account" for anyone who can measure it.
//
// It is built lazily rather than in init(), because a bcrypt derivation at
// cost 12 is a quarter of a second and every phpscript process would pay it
// whether or not the script ever asks about a password.
var (
	dummyHash []byte
	dummyOnce sync.Once
)

func timingDecoy(password string) {
	dummyOnce.Do(func() {
		dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), defaultCost)
	})
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
}

// Register installs the password hashing functions and their constants on rt.
func Register(rt *runner.Runtime) {
	rt.SetConst("PASSWORD_BCRYPT", algoBcrypt)
	rt.SetConst("PASSWORD_DEFAULT", algoBcrypt)
	rt.SetConst("PASSWORD_BCRYPT_DEFAULT_COST", int64(defaultCost))

	// password_hash returns a bcrypt hash of $password, salted from the
	// system CSPRNG. $algo is accepted and must name bcrypt, which is the
	// only algorithm implemented; $options takes a "cost" between 4 and 31.
	rt.RegisterFunc("password_hash", func(password string, opts ...any) (string, error) {
		cost, err := costFrom(opts)
		if err != nil {
			return "", fmt.Errorf("password_hash(): %w", err)
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
		if err != nil {
			return "", fmt.Errorf("password_hash(): %w", err)
		}
		return phpPrefix + strings.TrimPrefix(string(hash), goPrefix), nil
	})

	// password_verify reports whether $password produced $hash. A hash that
	// is empty or malformed is false, not an error: a login form asks a
	// question, and "no" is an answer.
	rt.RegisterFunc("password_verify", func(password string, hash string) bool {
		if hash == "" {
			timingDecoy(password)
			return false
		}
		return bcrypt.CompareHashAndPassword([]byte(goForm(hash)), []byte(password)) == nil
	})

	// password_needs_rehash reports whether $hash was made with a different
	// algorithm or cost than $algo and $options ask for, which is how a
	// login upgrades a stored hash without asking for the password twice.
	rt.RegisterFunc("password_needs_rehash", func(hash string, opts ...any) (bool, error) {
		want, err := costFrom(opts)
		if err != nil {
			return false, fmt.Errorf("password_needs_rehash(): %w", err)
		}

		got, err := bcrypt.Cost([]byte(goForm(hash)))
		if err != nil {
			// Not a bcrypt hash at all, so anything else is an upgrade.
			return true, nil
		}
		return got != want, nil
	})

	// password_get_info returns the algorithm and options $hash records, as
	// PHP's does: an unrecognised hash reports algo "" rather than failing.
	rt.RegisterFunc("password_get_info", func(hash string) *model.Array {
		info := model.NewArray()
		options := model.NewArray()

		cost, err := bcrypt.Cost([]byte(goForm(hash)))
		if err != nil {
			info.Set("algo", nil)
			info.Set("algoName", "unknown")
			info.Set("options", options)
			return info
		}

		options.Set("cost", int64(cost))
		info.Set("algo", algoBcrypt)
		info.Set("algoName", "bcrypt")
		info.Set("options", options)
		return info
	})
}

// goForm rewrites a PHP-written hash into the revision x/crypto accepts.
func goForm(hash string) string {
	if strings.HasPrefix(hash, phpPrefix) {
		return goPrefix + strings.TrimPrefix(hash, phpPrefix)
	}
	return hash
}

// costFrom reads the ($algo, $options) tail both hashing functions take. PHP
// allows the algorithm to be omitted in neither position, but it does allow
// either to be null, and a script that passes only options is the common case
// once PASSWORD_DEFAULT is the only algorithm on offer.
func costFrom(opts []any) (int, error) {
	cost := defaultCost

	for _, opt := range opts {
		switch value := opt.(type) {
		case nil:
			continue
		case *model.Array:
			found, err := costOption(value)
			if err != nil {
				return 0, err
			}
			cost = found
		case string:
			if value != algoBcrypt && value != "bcrypt" {
				return 0, fmt.Errorf("unsupported algorithm %q, only bcrypt is implemented", value)
			}
		case int64, int, float64:
			// PHP 7 spelled the algorithms as integers. Accept the
			// number and hash with bcrypt, which is what
			// PASSWORD_DEFAULT resolves to either way.
			continue
		default:
			return 0, fmt.Errorf("argument must be an algorithm or an options array, got %T", opt)
		}
	}

	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		return 0, fmt.Errorf("cost must be between %d and %d, got %d", bcrypt.MinCost, bcrypt.MaxCost, cost)
	}
	return cost, nil
}

// costOption reads options["cost"], which arrives as whatever the script wrote
// it as.
func costOption(options *model.Array) (int, error) {
	value, ok := options.Get("cost")
	if !ok {
		return defaultCost, nil
	}

	switch cost := value.(type) {
	case int64:
		return int(cost), nil
	case int:
		return cost, nil
	case float64:
		return int(cost), nil
	case string:
		parsed, err := strconv.Atoi(cost)
		if err != nil {
			return 0, fmt.Errorf("cost %q is not a number", cost)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("cost must be a number, got %T", value)
	}
}
