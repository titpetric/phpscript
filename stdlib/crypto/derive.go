package crypto

import (
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// The bcrypt cost x/crypto runs between. params.check refuses anything outside
// the range, so a script naming a cost the library would reject hears it in
// php's wording before a derivation starts.
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

// bcryptVerify reports whether password produced hash, passed to x/crypto as
// stored: it reads php's `$2y$` marker itself. The revision check is this
// package's own, because x/crypto reads markers php refuses.
func bcryptVerify(password, hash string) bool {
	if !bcryptRevision(hash) {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// bcryptRevision reports whether a stored hash opens with a revision marker
// php's crypt reads: $2a$, $2b$, $2x$ or $2y$. The 1997 "$2$" spelling with
// no minor revision parses in x/crypto and answers the failure string "*0"
// from php's crypt, as does a marker outside the four, so every bcrypt reader
// here requires one and refuses what php refuses:
//
//	php -r 'var_dump(password_verify("hunter2hunter2", "$2$04$..."));'  // bool(false)
//	php -r 'var_dump(password_verify("hunter2hunter2", "$2c$04$..."));' // bool(false)
func bcryptRevision(hash string) bool {
	return len(hash) >= 4 && hash[0] == '$' && hash[1] == '2' &&
		strings.IndexByte("abxy", hash[2]) >= 0 && hash[3] == '$'
}

// bcryptCost is the cost a stored hash carries, and an error for a
// string the library cannot read.
func bcryptCost(hash string) (int, error) {
	return bcrypt.Cost([]byte(hash))
}

// bcryptEncoded reports whether a stored hash is one a comparison could match.
//
// bcrypt.Cost reads the revision and the cost digits and stops, so a hash whose
// body is not bcrypt's base64 parses, reports a cost and then refuses every
// password. Checking the body tells a user database that such a line carries
// damage, where the cost alone would read as a wrong password.
//
// The width is exact because the format is: 7 bytes of prefix, 22 of salt and
// 31 of tag, over an alphabet that is bcrypt's own and not standard base64.
// The prefix carries a revision marker bcryptRevision reads, so a line the
// parse accepts is one the dispatch would also verify.
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

// argon2Key runs the variant p names. argon2i is data-independent and argon2id
// runs one data-independent pass before the data-dependent ones; both are
// reachable because php registers both, and argon2id is the one to choose.
func argon2Key(p params, password, salt []byte, keyLen uint32) []byte {
	if p.algo == algoArgon2i {
		return argon2.Key(password, salt, p.time, p.memory, p.lanes, keyLen)
	}
	return argon2.IDKey(password, salt, p.time, p.memory, p.lanes, keyLen)
}
