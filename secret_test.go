package jonson

import "testing"

func TestEncoding(t *testing.T) {
	key := "962C27B021AD53CC1110E81E6F6C09D7A14F7911C508A43A"
	enc := NewAESSecret(key)
	text := "Silvio"

	encoded := enc.Encode(text)
	if encoded == "" {
		t.Fatal("expected encoded result to not be empty")
	}
	if encoded == text {
		t.Fatal("expected encoded text not to equal original text")
	}

	decoded, err := enc.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if decoded != text {
		t.Fatal("expected decoded text to equal original text, got: " + decoded)
	}
}
