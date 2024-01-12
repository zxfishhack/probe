package probe

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"os"
)

var password = []byte("probe")

type message struct {
	ID        string `json:"id"`
	Signature string `json:"signature"`
}

func (m *message) signature() {
	m.Signature = calculateHMAC(password, m.ID)
	return
}

func (m *message) verify() (err error) {
	res := calculateHMAC(password, m.ID)
	if res != m.Signature {
		err = os.ErrInvalid
	}
	return
}

func calculateHMAC(hmacPassword []byte, parts ...string) (res string) {
	h := sha512.New()
	h.Write(append(hmacPassword, 'a'))
	hmacKey := h.Sum(nil)
	h = hmac.New(sha512.New, []byte(hex.EncodeToString(hmacKey)))
	for _, part := range parts {
		h.Write([]byte(part))
	}
	res = base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	return
}
