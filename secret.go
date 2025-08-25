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

	"golang.org/x/crypto/chacha20poly1305"
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
	Type() string
}

type OFBSecret struct {
	Shareable
	ShareableAcrossImpersonation
	cipher []byte
}

var _ Secret = (&OFBSecret{})

// NewOFBSecret uses the OFB cipher
//
// Deprecated: do no longer use
func NewOFBSecret(aesKeyHex string) *OFBSecret {
	aesCypher, err := hex.DecodeString(aesKeyHex)
	if err != nil {
		panic("error encoder: %w" + err.Error())
	}

	if len(aesCypher) != 16 && len(aesCypher) != 24 && len(aesCypher) != 32 {
		panic("error encoder: AES cypher needs to be 16, 24 or 32 bytes long, got: " + strconv.Itoa(len(aesCypher)))
	}

	return &OFBSecret{
		cipher: aesCypher,
	}
}

// Encode may be used to embed sensitive information
func (e *OFBSecret) Encode(in string) string {
	block, err := aes.NewCipher(e.cipher)
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

func (e *OFBSecret) Decode(in string) (string, error) {
	encoded, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		return "", err
	}
	if len(encoded) < aes.BlockSize {
		return "", errors.New("encoded text too short")
	}

	block, err := aes.NewCipher(e.cipher)
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

func (o *OFBSecret) Type() string {
	return "ofb"
}

type AEADSecret struct {
	Shareable
	ShareableAcrossImpersonation
	cipher cipher.AEAD
}

var _ Secret = (&AEADSecret{})

func NewAEADSecret(aesKeyHex string) *AEADSecret {
	aesCypher, err := hex.DecodeString(aesKeyHex)
	if err != nil {
		panic("error encoder: %w" + err.Error())
	}

	if len(aesCypher) != 32 {
		panic("error encoder: AEAD cypher needs to be 32 bytes long, got: " + strconv.Itoa(len(aesCypher)))
	}
	aead, err := chacha20poly1305.NewX(aesCypher)
	if err != nil {
		panic(err)
	}

	return &AEADSecret{
		cipher: aead,
	}
}

func (o *AEADSecret) Type() string {
	return "aead"
}

// Encode may be used to embed sensitive information
func (e *AEADSecret) Encode(_in string) string {
	in := []byte(_in)
	// Select a random nonce, and leave capacity for the ciphertext.
	nonce := make([]byte, e.cipher.NonceSize(), e.cipher.NonceSize()+len(in)+e.cipher.Overhead())
	if _, err := rand.Read(nonce); err != nil {
		panic(err)
	}

	// Encrypt the message and append the ciphertext to the nonce.
	encryptedMsg := e.cipher.Seal(nonce, nonce, in, nil)
	return base64.StdEncoding.EncodeToString(encryptedMsg)
}

func (e *AEADSecret) Decode(in string) (string, error) {
	encoded, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		return "", err
	}
	if len(encoded) < e.cipher.NonceSize() {
		return "", errors.New("encoded text too short")
	}

	// Split nonce and ciphertext.
	nonce, ciphertext := encoded[:e.cipher.NonceSize()], encoded[e.cipher.NonceSize():]

	// Decrypt the message and check it wasn't tampered with.
	out, err := e.cipher.Open(nil, nonce, ciphertext, nil)
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

func (d *DebugSecret) Type() string {
	return "debug"
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
