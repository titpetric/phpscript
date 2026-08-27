package crypto

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

	"github.com/titpetric/phpscript/runner"
)

// RegisterRandom installs the CSPRNG functions on rt. They are the PHP 7+
// core pair (paragonie/random_compat existed solely to polyfill them), read
// from crypto/rand, the same source Session\Manager mints session ids from.
// The seeded family (mt_rand, rand, srand) is deliberately absent: PHP itself
// points modern code at random_int, and nothing here wants reproducible
// randomness.
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
		if min == max {
			return min, nil
		}
		// The span max-min+1 overflows int64 when the bounds cover most of
		// the type's range, so it is computed in big.Int; rand.Int does the
		// rejection sampling that keeps the pick unbiased.
		span := new(big.Int).Sub(big.NewInt(max), big.NewInt(min))
		span.Add(span, big.NewInt(1))
		n, err := rand.Int(rand.Reader, span)
		if err != nil {
			return 0, fmt.Errorf("random_int(): %w", err)
		}
		return n.Add(n, big.NewInt(min)).Int64(), nil
	})
}
