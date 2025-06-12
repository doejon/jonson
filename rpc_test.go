package jonson

import (
	"context"
	"encoding/json"
	"testing"
)

type TestDataUnknownFields struct {
	Uuid string `json:"id"`
}

func (t *TestDataUnknownFields) JonsonAllowUnknownFields() {
	// do nothing
}

var _ AllowUnknownFieldsParams = (&TestDataUnknownFields{})

func TestUnmarshalAndValidate(t *testing.T) {
	type Data1 struct {
		Uuid string `json:"id"`
	}

	data1 := &Data1{
		Uuid: "f82eecec-70c2-45b0-ad0c-07a982505992",
	}

	bdata1, _ := json.Marshal(data1)

	type Data2 struct {
		Uuid  string `json:"uuid"`
		Uuid2 string `json:"uuid2"`
	}

	data2 := &Data2{
		Uuid:  "f82eecec-70c2-45b0-ad0c-07a982505992",
		Uuid2: "002cc5c2-6d42-41b5-8bd5-9448d7a4e44e",
	}

	bdata2, _ := json.Marshal(data2)

	req1 := &RpcRequest{
		Params: bdata1,
	}

	req2 := &RpcRequest{
		Params: bdata2,
	}

	fac := NewFactory()
	fac.RegisterProvider(newJsonHandlerProvider(NewDefaultJsonHandler()))

	ctx := NewContext(context.Background(), fac, nil)
	sec := NewDebugSecret()

	t.Run("unmarshaling succeeds", func(t *testing.T) {

		out1 := &Data1{}
		err := req1.UnmarshalAndValidate(ctx, sec, out1)
		if err != nil {
			t.Fatal("expected no error", err)
		}
		if out1.Uuid != data1.Uuid {

		}

		out2 := &Data2{}
		err = req2.UnmarshalAndValidate(ctx, sec, out2)
		if err != nil {
			t.Fatal("expected no error", err)
		}

		if out2.Uuid != data2.Uuid {
			t.Fatal("expected uuids to be equal")
		}

		if out2.Uuid2 != data2.Uuid2 {
			t.Fatal("expected uuids to be equal")
		}
	})

	t.Run("unmarshaling fails with unknown fields", func(t *testing.T) {

		out1 := &Data1{}
		err := req2.UnmarshalAndValidate(ctx, sec, out1)
		if err == nil {
			t.Fatal("unknown fields must fail")
		}
	})

	t.Run("unmarshaling with unknown fields succeeds in case the data type implements AllowUnknownFieldsParams", func(t *testing.T) {

		out1 := &TestDataUnknownFields{}
		err := req2.UnmarshalAndValidate(ctx, sec, out1)
		if err != nil {
			t.Fatal("unknown fields are allowed")
		}
	})
}
