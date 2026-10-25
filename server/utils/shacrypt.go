package utils

import (
	"crypto/sha512"
	"strconv"
	"strings"
)

const (
	shaCryptDefaultRounds = 5000
	shaCryptMinRounds     = 1000
	shaCryptMaxRounds     = 999999999
	shaCryptMaxSalt       = 16
	shaCryptAlphabet      = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

// shaCryptOrder is the byte order of the SHA-512 crypt base64 output: each
// triple (a, b, c) is encoded as the 24-bit value a<<16 | b<<8 | c, low six
// bits first. The final byte (63) is encoded on its own as two characters.
var shaCryptOrder = [21][3]int{
	{0, 21, 42}, {22, 43, 1}, {44, 2, 23}, {3, 24, 45}, {25, 46, 4},
	{47, 5, 26}, {6, 27, 48}, {28, 49, 7}, {50, 8, 29}, {9, 30, 51},
	{31, 52, 10}, {53, 11, 32}, {12, 33, 54}, {34, 55, 13}, {56, 14, 35},
	{15, 36, 57}, {37, 58, 16}, {59, 17, 38}, {18, 39, 60}, {40, 61, 19},
	{62, 20, 41},
}

// SHA512Crypt returns the SHA-512 crypt ($6$) hash of password, following
// Ulrich Drepper's "Unix crypt using SHA-256 and SHA-512". The salt is cut at
// the first '$' and truncated to 16 characters. rounds 0 means the default
// 5000 and leaves the "rounds=" prefix out; any other value is clamped to
// 1000..999999999 and written as "$6$rounds=N$...".
func SHA512Crypt(password, salt string, rounds int) string {
	explicit := rounds != 0
	if rounds == 0 {
		rounds = shaCryptDefaultRounds
	}
	if rounds < shaCryptMinRounds {
		rounds = shaCryptMinRounds
	}
	if rounds > shaCryptMaxRounds {
		rounds = shaCryptMaxRounds
	}
	if i := strings.IndexByte(salt, '$'); i >= 0 {
		salt = salt[:i]
	}
	if len(salt) > shaCryptMaxSalt {
		salt = salt[:shaCryptMaxSalt]
	}

	pw := []byte(password)
	sl := []byte(salt)

	// Steps 4-8: digest B = H(password salt password).
	b := sha512.New()
	b.Write(pw)
	b.Write(sl)
	b.Write(pw)
	sumB := b.Sum(nil)

	// Steps 1-3, 9-12: digest A.
	a := sha512.New()
	a.Write(pw)
	a.Write(sl)
	n := len(pw)
	for ; n > sha512.Size; n -= sha512.Size {
		a.Write(sumB)
	}
	a.Write(sumB[:n])
	for n = len(pw); n > 0; n >>= 1 {
		if n&1 != 0 {
			a.Write(sumB)
		} else {
			a.Write(pw)
		}
	}
	sumA := a.Sum(nil)

	// Steps 13-15: DP = H(password repeated len(password) times); P is DP
	// repeated or cut to len(password) bytes.
	dp := sha512.New()
	for i := 0; i < len(pw); i++ {
		dp.Write(pw)
	}
	p := repeatTo(dp.Sum(nil), len(pw))

	// Steps 16-18: DS = H(salt repeated 16 + A[0] times); S is DS cut to
	// len(salt) bytes.
	ds := sha512.New()
	for i := 0; i < 16+int(sumA[0]); i++ {
		ds.Write(sl)
	}
	s := repeatTo(ds.Sum(nil), len(sl))

	// Step 21: the rounds.
	sum := sumA
	for i := 0; i < rounds; i++ {
		c := sha512.New()
		if i&1 != 0 {
			c.Write(p)
		} else {
			c.Write(sum)
		}
		if i%3 != 0 {
			c.Write(s)
		}
		if i%7 != 0 {
			c.Write(p)
		}
		if i&1 != 0 {
			c.Write(sum)
		} else {
			c.Write(p)
		}
		sum = c.Sum(nil)
	}

	// Step 22: encode.
	var out strings.Builder
	out.WriteString("$6$")
	if explicit {
		out.WriteString("rounds=" + strconv.Itoa(rounds) + "$")
	}
	out.WriteString(salt)
	out.WriteByte('$')
	for _, t := range shaCryptOrder {
		w := uint32(sum[t[0]])<<16 | uint32(sum[t[1]])<<8 | uint32(sum[t[2]])
		writeB64(&out, w, 4)
	}
	writeB64(&out, uint32(sum[63]), 2)
	return out.String()
}

// repeatTo returns n bytes made of src repeated, cut at n.
func repeatTo(src []byte, n int) []byte {
	out := make([]byte, 0, n)
	for len(out) < n {
		take := n - len(out)
		if take > len(src) {
			take = len(src)
		}
		out = append(out, src[:take]...)
	}
	return out
}

// writeB64 writes n characters of w in the crypt alphabet, low six bits first.
func writeB64(out *strings.Builder, w uint32, n int) {
	for ; n > 0; n-- {
		out.WriteByte(shaCryptAlphabet[w&0x3f])
		w >>= 6
	}
}
