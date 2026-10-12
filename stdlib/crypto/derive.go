package crypto

import (
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// The bcrypt cost x/crypto runs between. validate refuses anything outside the
// range, so a script naming a cost the library would reject is told in php's
// wording before a derivation starts.
const (
	bcryptMinCost = bcrypt.MinCost
	bcryptMaxCost = bcrypt.MaxCost
)

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

// bcryptVerify reports whether password produced hash, reading the cost and the
// salt out of the hash the way argon2Verify does.
func bcryptVerify(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(goForm(hash)), []byte(password)) == nil
}

// bcryptCost is the cost a stored hash was written at, and an error for a
// string the library cannot read.
func bcryptCost(hash string) (int, error) {
	return bcrypt.Cost([]byte(goForm(hash)))
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
