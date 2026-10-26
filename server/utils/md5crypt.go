package utils

import (
	"crypto/md5"
	"strings"
)

const md5CryptMaxSalt = 8

// MD5Crypt returns the MD5-crypt ($1$) hash of password (Poul-Henning Kamp's
// FreeBSD scheme, what busybox passwd and openssl passwd -1 write). The salt
// is cut at the first '$' and truncated to 8 characters. It exists only to
// verify hashes the stock image already holds; new passwords are SHA-512.
func MD5Crypt(password, salt string) string {
	if i := strings.IndexByte(salt, '$'); i >= 0 {
		salt = salt[:i]
	}
	if len(salt) > md5CryptMaxSalt {
		salt = salt[:md5CryptMaxSalt]
	}
	pw := []byte(password)
	sl := []byte(salt)

	alt := md5.New()
	alt.Write(pw)
	alt.Write(sl)
	alt.Write(pw)
	sumAlt := alt.Sum(nil)

	ctx := md5.New()
	ctx.Write(pw)
	ctx.Write([]byte("$1$"))
	ctx.Write(sl)
	for n := len(pw); n > 0; n -= md5.Size {
		take := n
		if take > md5.Size {
			take = md5.Size
		}
		ctx.Write(sumAlt[:take])
	}
	for n := len(pw); n > 0; n >>= 1 {
		if n&1 != 0 {
			ctx.Write([]byte{0})
		} else if len(pw) > 0 {
			ctx.Write(pw[:1])
		}
	}
	sum := ctx.Sum(nil)

	for i := 0; i < 1000; i++ {
		c := md5.New()
		if i&1 != 0 {
			c.Write(pw)
		} else {
			c.Write(sum)
		}
		if i%3 != 0 {
			c.Write(sl)
		}
		if i%7 != 0 {
			c.Write(pw)
		}
		if i&1 != 0 {
			c.Write(sum)
		} else {
			c.Write(pw)
		}
		sum = c.Sum(nil)
	}

	var out strings.Builder
	out.WriteString("$1$")
	out.WriteString(salt)
	out.WriteByte('$')
	for _, t := range [][3]int{{0, 6, 12}, {1, 7, 13}, {2, 8, 14}, {3, 9, 15}, {4, 10, 5}} {
		w := uint32(sum[t[0]])<<16 | uint32(sum[t[1]])<<8 | uint32(sum[t[2]])
		writeB64(&out, w, 4)
	}
	writeB64(&out, uint32(sum[11]), 2)
	return out.String()
}
