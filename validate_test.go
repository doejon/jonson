package jonson

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

type Profile struct {
	Name          string `json:"name"`
	Image         *Image `json:"image,omitempty"`
	ImageRequired Image  `json:"imageRequired"`

	NotExposed string `json:"-"`
}

type Image struct {
	URL  string
	UUID string
}

func (i *Image) ValidateURL() error {
	if len(i.URL) < 1 {
		return fmt.Errorf("no url given")
	}
	return nil
}

func (i *Image) ValidateUUID() error {
	if len(i.UUID) < 1 {
		return fmt.Errorf("UUID too short")
	}
	return nil
}

func (t *Profile) ValidateName() error {
	if len(t.Name) < 1 {
		return errors.New("uuid too short")
	}
	return nil
}

func TestValidate(t *testing.T) {
	profile := &Profile{}
	t.Run("expect profile to implement validator", func(t *testing.T) {
		err := implementsValidator(reflect.TypeOf(profile), nil)
		if err != nil {
			t.Fatal(errors.Join(err...))
		}
	})
}
