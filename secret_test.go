package jonson

import "testing"

func TestAESSecret(t *testing.T) {
	key := "962C27B021AD53CC1110E81E6F6C09D7A14F7911C508A43A"
	enc := NewOFBSecret(key)
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

	if enc.Type() != "ofb" {
		t.Fatal("expected type to equal ofb")
	}
}

func TestAEADSecret(t *testing.T) {
	key := "962C27B021AD53CC1110E81E6F6C09D7A14F7911C508A43AFBA4CFAF14543156"
	enc := NewAEADSecret(key)

	result := map[string]struct{}{}

	for i := 0; i < 5; i++ {
		text := "Silvio"

		encoded := enc.Encode(text)
		if encoded == "" {
			t.Fatal("expected encoded result to not be empty")
		}

		if encoded == text {
			t.Fatal("expected encoded text not to equal original text")
		}

		if _, ok := result[encoded]; ok {
			t.Fatal("found encoded result twice - issue with nonce?")
		}

		decoded, err := enc.Decode(encoded)
		if err != nil {
			t.Fatal(err)
		}

		if decoded != text {
			t.Fatal("expected decoded text to equal original text, got: " + decoded)
		}
	}

	if enc.Type() != "aead" {
		t.Fatal("expected type to equal aead")
	}

}

func TestDebugSecret(t *testing.T) {
	enc := NewDebugSecret()
	text := "Silvio"

	encoded := enc.Encode(text)

	if encoded != text {
		t.Fatal("expected encoded text to equal original text")
	}

	decoded, err := enc.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if decoded != text {
		t.Fatal("expected decoded text to equal original text, got: " + decoded)
	}
}
