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

var TypeSecret = reflect.TypeOf((*Secret)(nil)).Elem()

func RequireSecret(ctx *Context) Secret {
	if v := ctx.Require(TypeSecret); v != nil {
		return v.(Secret)
	}
	return nil
}

type Secret interface {
	Shareable
	ShareableAcrossImpersonation
	Encode(in string) string
	Decode(in string) (string, error)
}

type AESSecret struct {
	Shareable
	ShareableAcrossImpersonation
	aesCypher []byte
}

var _ Secret = (&AESSecret{})

func NewAESSecret(aesKeyHex string) *AESSecret {
	aesCypher, err := hex.DecodeString(aesKeyHex)
	if err != nil {
		panic("error encoder: %w" + err.Error())
	}

	if len(aesCypher) != 16 && len(aesCypher) != 24 && len(aesCypher) != 32 {
		panic("error encoder: AES cypher needs to be 16, 24 or 32 bytes long, got: " + strconv.Itoa(len(aesCypher)))
	}

	return &AESSecret{
		aesCypher: aesCypher,
	}
}

// Encode may be used to embed sensitive information
func (e *AESSecret) Encode(in string) string {
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

func (e *AESSecret) Decode(in string) (string, error) {
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

// DebugSecret implements the Secret interface without
// actually encrypting the passed data: the data will be returned
// as-is. The functionality can be helpful for debugging certain
// error messages during development.
type DebugSecret struct {
	Shareable
	ShareableAcrossImpersonation
}

var _ Secret = (&DebugSecret{})

func NewDebugSecret() *DebugSecret {
	return &DebugSecret{}
}

// Encode may be used to embed sensitive information
func (e *DebugSecret) Encode(in string) string {
	return in
}

func (e *DebugSecret) Decode(in string) (string, error) {
	return in, nil
}

// secretProvider is used internally to provide the secret
// once it's passed down to the method handler
type secretProvider struct {
	secret Secret
}

func newSecretProvider(secret Secret) *secretProvider {
	return &secretProvider{
		secret: secret,
	}
}

func (s *secretProvider) NewSecret(ctx *Context) Secret {
	return s.secret
}
