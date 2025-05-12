package jonson

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

type Profile struct {
	Name          string   `json:"name"`
	Image         *Image   `json:"image,omitempty"`
	ImageRequired Image    `json:"imageRequired"`
	ImageArr      []*Image `json:"imageArray"`
	BirthdayTs    int64    `json:"birthday"`
}

func (p *Profile) JonsonValidate(v *Validator) {
	if len(p.Name) < 2 || len(p.Name) > 10 {
		v.Path("name").Code(-1).Debug("secret debug message").Message("name len between 2 and 10 chars")
	}

	if p.Image != nil {
		v.Path("image").Validate(p.Image)
	}

	v.Path("imageRequired").Validate(&p.ImageRequired)

	for idx, img := range p.ImageArr {
		v.Path("imageArr", v.Index(idx)).Validate(img)
	}

	tm := RequireTime(v.Context).Now().Unix()
	if p.BirthdayTs < tm {
		v.Path("birthdayTs").Message(fmt.Sprintf("birthday before or equal timestamp, got: %d", tm))
	}

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
	fac := NewFactory()
	tm := time.Unix(1000, 0)
	fac.RegisterProvider(NewTimeProvider(func() Time {
		return newMockTime(tm)
	}))

	ctx := NewContext(context.Background(), fac, nil)

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
			ImageArr:      []*Image{validImage(), validImage()},
			BirthdayTs:    tm.Unix() + 1,
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
			name: "birthday invalid",
			data: func() *Profile {
				out := validProfile()
				out.BirthdayTs = 0
				return out
			},
			inspect: func(e *Error) error {
				if e == nil {
					return fmt.Errorf("error expected")
				}
				if e.Data.Details[0].Data.Path[0] != "birthdayTs" {
					return fmt.Errorf("expected 'name' to have an error")
				}
				if e.Data.Details[0].Code != -32602 {
					return fmt.Errorf("expected code to equal -32602, got: %d", e.Data.Details[0].Code)
				}
				if e.Data.Details[0].Message != "birthday before or equal timestamp, got: 1000" {
					return fmt.Errorf("expected message to be equal, got: %s", e.Data.Details[0].Message)
				}

				if err, _ := e.Inspect().Code(-32602).Path("birthdayTs").FindFirst(); err == nil {
					t.Fatal("expected to find -32602 error with birthdayTs as a path")
				}

				errs := e.Inspect().Code(-32602).Path("birthdayTs").FindAll()
				if len(errs) != 1 {
					t.Fatalf("expected to find -32602 with birthdayTs exactly once, got: %d", len(errs))
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
		{
			name: "image in array invalid",
			data: func() *Profile {
				out := validProfile()
				out.ImageArr[1].URL = ""
				return out
			},
			inspect: func(e *Error) error {
				if e == nil {
					return fmt.Errorf("error expected")
				}
				paths := e.Data.Details[0].Data.Path

				if paths[0] != "imageArr" {
					return fmt.Errorf("expected 'requiredImage' to be first in paths")
				}
				if paths[1] != "[1]" {
					return fmt.Errorf("expected 'index' to second in paths")
				}
				if paths[2] != "url" {
					return fmt.Errorf("expected 'url' to be third in paths")
				}
				return nil
			},
		},
	}

	for _, v := range tests {
		t.Run(v.name, func(t *testing.T) {
			data := v.data()
			secret := NewDebugSecret()
			result := Validate(ctx, secret, data)
			if err := v.inspect(result); err != nil {
				t.Fatal(err)
			}
		})
	}

}
