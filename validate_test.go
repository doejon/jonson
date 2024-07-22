package jonson

import (
	"fmt"
	"strings"
	"testing"
)

type Profile struct {
	Name          string `json:"name"`
	Image         *Image `json:"image,omitempty"`
	ImageRequired Image  `json:"imageRequired"`
}

func (p *Profile) JonsonValidate(v *Validator) {
	if len(p.Name) < 2 || len(p.Name) > 10 {
		v.Path("name").Code(-1).Debug("secret debug message").Message("name len between 2 and 10 chars")
	}

	if p.Image != nil {
		v.Path("image").Validate(p.Image)
	}

	v.Path("imageRequired").Validate(&p.ImageRequired)
}

type Image struct {
	URL  string
	UUID string
}

func (i *Image) JonsonValidate(v *Validator) {
	if len(i.URL) < 1 {
		v.Path("url").Message("url too short")
	}
	if len(i.URL) > 20 {
		v.Path("url").Message("url too long")
	}
	if len(i.UUID) != 36 {
		v.Path("uuid").Message("uuid invalid")
	}
}

func TestValidate(t *testing.T) {
	validImage := func() *Image {
		return &Image{
			UUID: "d69b8e2c-3e72-47fe-9c06-5113d03e7d59",
			URL:  "https://example.com",
		}
	}
	validProfile := func() *Profile {
		i := validImage()
		return &Profile{
			Name:          "Silvio",
			Image:         validImage(),
			ImageRequired: *i,
		}
	}

	type test struct {
		name    string
		data    func() *Profile
		inspect func(e *Error) error
	}

	tests := []*test{
		{
			name: "valid profile",
			data: func() *Profile {
				return validProfile()
			},
			inspect: func(e *Error) error {
				if e != nil {
					return fmt.Errorf("error must be nil")
				}
				return nil
			},
		},
		{
			name: "name invalid",
			data: func() *Profile {
				out := validProfile()
				out.Name = strings.Repeat("a", 11)
				return out
			},
			inspect: func(e *Error) error {
				if e == nil {
					return fmt.Errorf("error expected")
				}
				if e.Data.Details[0].Data.Path[0] != "name" {
					return fmt.Errorf("expected 'name' to have an error")
				}
				if e.Data.Details[0].Code != -1 {
					return fmt.Errorf("expected code to equal -1, got: %d", e.Data.Details[0].Code)
				}
				if e.Data.Details[0].Data.Debug != "secret debug message" {
					return fmt.Errorf("expected secret debug message to be set")
				}
				return nil
			},
		},
		{
			name: "image invalid",
			data: func() *Profile {
				out := validProfile()
				out.ImageRequired.URL = ""
				return out
			},
			inspect: func(e *Error) error {
				if e == nil {
					return fmt.Errorf("error expected")
				}
				paths := e.Data.Details[0].Data.Path

				if paths[0] != "imageRequired" {
					return fmt.Errorf("expected 'requiredImage' to be first in paths")
				}
				if paths[1] != "url" {
					return fmt.Errorf("expected 'url' to be second in paths")
				}
				return nil
			},
		},
	}

	for _, v := range tests {
		t.Run(v.name, func(t *testing.T) {
			data := v.data()
			secret := NewDebugSecret()
			result := Validate(secret, data)
			if err := v.inspect(result); err != nil {
				t.Fatal(err)
			}
		})
	}

}
