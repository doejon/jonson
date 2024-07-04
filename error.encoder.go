package jonson

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"reflect"
	"strconv"
)

var TypeErrorEncoder = reflect.TypeOf((*ErrorEncoder)(nil)).Elem()

func RequireErrorEncoder(ctx *Context) ErrorEncoder {
	if v := ctx.Require(TypeErrorEncoder); v != nil {
		return v.(ErrorEncoder)
	}
	return nil
}

type ErrorEncoder interface {
	Encode(in string) string
}

type AESErrorEncoder struct {
	aesCypher []byte
}

func NewAESErrorEncoder(aesKeyHex string) *AESErrorEncoder {
	aesCypher, err := hex.DecodeString(aesKeyHex)
	if err != nil {
		panic("error encoder: %w" + err.Error())
	}

	if len(aesCypher) != 16 && len(aesCypher) != 24 && len(aesCypher) != 32 {
		panic("error encoder: AES cypher needs to be 16, 24 or 32 bytes long, got: " + strconv.Itoa(len(aesCypher)))
	}

	return &AESErrorEncoder{
		aesCypher: aesCypher,
	}
}

// Encode may be used to embed sensitive information
func (e *AESErrorEncoder) Encode(in string) string {
	block, err := aes.NewCipher(e.aesCypher)
	if err != nil {
		return ""
	}

	plaintext := []byte(in)
	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return ""
	}
	stream := cipher.NewOFB(block, iv[:])

	stream.XORKeyStream(ciphertext[aes.BlockSize:], plaintext)

	return base64.StdEncoding.EncodeToString(ciphertext)
}

func (e *AESErrorEncoder) Decode(in string) (string, error) {
	encoded, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		return "", err
	}
	if len(encoded) < aes.BlockSize {
		return "", errors.New("encoded text too short")
	}

	block, err := aes.NewCipher(e.aesCypher)
	if err != nil {
		return "", err
	}

	stream := cipher.NewOFB(block, encoded[:aes.BlockSize])

	reader := &cipher.StreamReader{S: stream, R: bytes.NewReader(encoded[aes.BlockSize:])}

	out, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
