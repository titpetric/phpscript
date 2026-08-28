package crypto

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

	"github.com/titpetric/phpscript/runner"
)

// RegisterRandom installs the random functions on rt. random_bytes and
// random_int are the PHP 7+ core pair (paragonie/random_compat existed solely
// to polyfill them), read from crypto/rand, the same source Session\Manager
// mints session ids from. rand reads the same source: it exists because the
// PHP written against it is everywhere, but it takes no seed, so the seeded
// reproducibility of srand/mt_srand stays absent and those two with it.
func RegisterRandom(rt *runner.Runtime) {
	// random_bytes returns $length cryptographically secure random bytes, throwing when $length is less than 1.
	rt.RegisterFunc("random_bytes", func(length int64) (string, error) {
		if length < 1 {
			return "", errors.New("random_bytes(): Argument #1 ($length) must be greater than 0")
		}
		buf := make([]byte, length)
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("random_bytes(): %w", err)
		}
		return string(buf), nil
	})

	// random_int returns a cryptographically secure, uniformly selected integer between $min and $max inclusive, throwing when $min is greater than $max.
	rt.RegisterFunc("random_int", func(min, max int64) (int64, error) {
		if min > max {
			return 0, errors.New("random_int(): Argument #1 ($min) must be less than or equal to Argument #2 ($max)")
		}
		return uniformInt64("random_int", min, max)
	})

	// rand returns a uniformly selected integer between $min and $max inclusive, or between 0 and 2147483647 when called without arguments; it reads the CSPRNG rather than a seeded generator, so there is no srand to pair it with.
	rt.RegisterFunc("rand", func(bounds ...int64) (int64, error) {
		min, max := int64(0), int64(randMax)
		switch len(bounds) {
		case 0:
		case 2:
			min, max = bounds[0], bounds[1]
			if min > max {
				return 0, errors.New("rand(): Argument #1 ($min) must be less than or equal to Argument #2 ($max)")
			}
		default:
			return 0, errors.New("rand() expects exactly 0 or 2 arguments")
		}
		return uniformInt64("rand", min, max)
	})
}

// randMax is PHP's getrandmax(), the upper bound of an argument-less rand().
const randMax = 2147483647

// uniformInt64 picks from [min, max] inclusive. The span max-min+1 overflows
// int64 when the bounds cover most of the type's range, so it is computed in
// big.Int; rand.Int does the rejection sampling that keeps the pick unbiased.
func uniformInt64(name string, min, max int64) (int64, error) {
	if min == max {
		return min, nil
	}
	span := new(big.Int).Sub(big.NewInt(max), big.NewInt(min))
	span.Add(span, big.NewInt(1))
	n, err := rand.Int(rand.Reader, span)
	if err != nil {
		return 0, fmt.Errorf("%s(): %w", name, err)
	}
	return n.Add(n, big.NewInt(min)).Int64(), nil
}
