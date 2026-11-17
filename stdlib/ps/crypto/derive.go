package crypto

import (
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// bcryptMinCost and bcryptMaxCost bound the cost params.check accepts.
const (
	bcryptMinCost = bcrypt.MinCost
	bcryptMaxCost = bcrypt.MaxCost
)

// derive runs p's algorithm, rewriting bcrypt's revision marker to php's.
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

// bcryptVerify reports whether password produced hash, checking php's revision marker first.
func bcryptVerify(password, hash string) bool {
	if !bcryptRevision(hash) {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// bcryptRevision reports whether a stored hash opens with $2a$, $2b$, $2x$ or $2y$.
func bcryptRevision(hash string) bool {
	return len(hash) >= 4 && hash[0] == '$' && hash[1] == '2' &&
		strings.IndexByte("abxy", hash[2]) >= 0 && hash[3] == '$'
}

// bcryptCost returns the cost a stored hash records, and an error for a hash it cannot read.
func bcryptCost(hash string) (int, error) {
	return bcrypt.Cost([]byte(hash))
}

// bcryptEncoded reports whether a stored hash has the full 60-byte bcrypt body a comparison needs.
func bcryptEncoded(hash string) bool {
	const (
		bcryptLen      = 60
		bcryptBodyAt   = 7
		bcryptAlphabet = "./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	)

	if len(hash) != bcryptLen || !bcryptRevision(hash) || hash[6] != '$' {
		return false
	}
	if hash[4] < '0' || hash[4] > '9' || hash[5] < '0' || hash[5] > '9' {
		return false
	}
	return strings.IndexFunc(hash[bcryptBodyAt:], func(r rune) bool {
		return !strings.ContainsRune(bcryptAlphabet, r)
	}) < 0
}

// argon2Key runs the argon2 variant p names, argon2i or argon2id.
func argon2Key(p params, password, salt []byte, keyLen uint32) []byte {
	if p.algo == algoArgon2i {
		return argon2.Key(password, salt, p.time, p.memory, p.lanes, keyLen)
	}
	return argon2.IDKey(password, salt, p.time, p.memory, p.lanes, keyLen)
}
