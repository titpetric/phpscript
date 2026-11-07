package bindings

import (
	"slices"
)

// A binding binds a package this build already links. Binding one it does not
// grows the binary and the dependency surface of every host that embeds the
// runtime, which is a decision and not a side effect of adding a function.
//
// The list below is what the released binary links today, read off the symbols
// and sanitised: the internal trees come out, as do the packages that serve
// the toolchain and no program.
//
//	go tool nm $(which phpscript) \
//	  | awk '{print $3}' \
//	  | grep -oE "^[a-z][a-z0-9]*(/[a-z0-9_]+)*\." \
//	  | sed 's/\.$//' | sort -u
//
// `go list -deps .` answers the same question from the source tree. A package
// outside the list is an addition to argue for, in the change that binds it.

// linked is the standard library subset this build links, sorted.
var linked = []string{
	"bufio", "bytes", "cmp",
	"compress/flate", "compress/gzip", "compress/lzw", "compress/zlib",
	"context",
	"crypto", "crypto/aes", "crypto/cipher", "crypto/des", "crypto/ecdh",
	"crypto/ecdsa", "crypto/ed25519", "crypto/elliptic", "crypto/hkdf",
	"crypto/hmac", "crypto/hpke", "crypto/md5", "crypto/mldsa", "crypto/mlkem",
	"crypto/pbkdf2", "crypto/rand", "crypto/rc4", "crypto/rsa", "crypto/sha1",
	"crypto/sha256", "crypto/sha3", "crypto/sha512", "crypto/tls",
	"crypto/x509",
	"database/sql",
	"encoding", "encoding/asn1", "encoding/base32", "encoding/base64",
	"encoding/binary", "encoding/csv", "encoding/hex", "encoding/json",
	"encoding/pem", "encoding/xml",
	"errors", "flag", "fmt",
	"hash", "hash/adler32", "hash/crc32",
	"html", "html/template",
	"image", "image/color", "image/draw", "image/gif", "image/jpeg",
	"image/png",
	"io", "io/fs", "iter",
	"log", "log/slog",
	"maps", "math", "math/big", "math/rand",
	"mime", "mime/multipart", "mime/quotedprintable",
	"net", "net/http", "net/mail", "net/netip", "net/smtp", "net/textproto",
	"net/url",
	"os", "os/exec",
	"path", "path/filepath",
	"reflect", "regexp",
	"slices", "sort", "strconv", "strings",
	"sync", "sync/atomic",
	"text/tabwriter", "text/template",
	"time",
	"unicode", "unicode/utf16", "unicode/utf8",
}

// Linked reports whether the build links path, which a binding may bind.
func Linked(path string) bool {
	return slices.Contains(linked, path)
}

// LinkedPackages lists the standard library subset a binding may bind.
func LinkedPackages() []string {
	return slices.Clone(linked)
}
