package cidre

import (
	"reflect"
	"testing"
)

type testDynamicStruct1 struct { }
type testDynamicStruct2 struct { }

func TestDynamicObjectFactory(t *testing.T) {
  DynamicObjectFactory.Register(testDynamicStruct1{})
  errorIfNotEqual(t, reflect.TypeOf(&testDynamicStruct1{}).String(),
    reflect.TypeOf(DynamicObjectFactory.New("cidre.testDynamicStruct1")).String())
}

func TestBuildString(t *testing.T) {
  errorIfNotEqual(t, "ABCDE", BuildString(10, "A", "B", "C", "D", "E"))
}

func TestSignedString(t *testing.T) {
  str := "ABCDE"
  secret := "secret"
  signed := SignString(str, secret)
  if decoded, err := ValidateSignedString(signed, secret); err != nil {
    t.Errorf("err must be nil")
  } else {
    errorIfNotEqual(t, str, decoded)
  }
  tampered := "FGH" + signed[5:]
  if _, err := ValidateSignedString(tampered, secret); err == nil {
    t.Errorf("data has been tampered, but err is nil")
  }
}
