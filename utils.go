package cidre

import (
	"crypto/hmac"
	"crypto/sha1"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

/* DynamicObjectFactory {{{ */

type dynamicObjectFactory map[string]reflect.Type

var dynamicObjectFactoryCh = make(chan bool, 1)

// DynamicObjectFactory provides functions to create an object by string name.
//
//     package mypackage
//
//     type MyObject struct {}
//     DynamicObjectFactory.Register(MyObject{})
//     DynamicObjectFactory.New("mypackage.MyObject")
var DynamicObjectFactory = make(dynamicObjectFactory)

func (self dynamicObjectFactory) Register(infs ...interface{}) {
	dynamicObjectFactoryCh <- true
	defer func() { <-dynamicObjectFactoryCh }()
	for _, inf := range infs {
		typ := reflect.TypeOf(inf)
		self[typ.String()] = typ
	}
}

func (self dynamicObjectFactory) New(name string) interface{} {
	dynamicObjectFactoryCh <- true
	defer func() { <-dynamicObjectFactoryCh }()
	typ, ok := self[name]
	if !ok {
		panic("DynamicObjectFactory: type name " + name + " not found.")
	}
	return reflect.New(typ).Interface()
}

// }}}

/* {{{ */

// Dict is a Python's dict like object.
type Dict map[string]interface{}

func NewDict() Dict {
	return make(Dict)
}

func (self Dict) Update(other map[string]interface{}) {
	for key, value := range other {
		self[key] = value
	}
}

func (self Dict) Copy(other map[string]interface{}) {
	Dict(other).Update(self)
}

func (self Dict) Pop(key string) interface{} {
	v := self.Get(key)
	self.Del(key)
	return v
}

func (self Dict) Get(key string) interface{} {
	return self[key]
}

func (self Dict) GetOr(key string, value interface{}) interface{} {
	if self.Has(key) {
		return self.Get(key)
	} else {
		return value
	}
}

func (self Dict) Has(key string) bool {
	_, ok := self[key]
	return ok
}

func (self Dict) GetString(key string) string {
	if v, ok := self[key]; ok {
		return v.(string)
	} else {
		return ""
	}
}

func (self Dict) GetInt(key string) int {
	if v, ok := self[key]; ok {
		return v.(int)
	} else {
		return 0
	}
}

func (self Dict) GetBool(key string) bool {
	if v, ok := self[key]; ok {
		return v.(bool)
	} else {
		return false
	}
}

func (self Dict) Set(key string, value interface{}) Dict {
	self[key] = value
	return self
}

func (self Dict) Del(key string) Dict {
	delete(self, key)
	return self
}

//}}}

// String utils {{{

// Returns a string that is the concatenation of the strings in efficient way.
func BuildString(ca int, ss ...string) string {
	buf := make([]byte, 0, ca)
	for _, s := range ss {
		buf = append(buf, s...)
	}
	return string(buf)
}

// Returns a string with a HMAC signature.
func SignString(value, key string) string {
	return fmt.Sprintf("%x----%s", hmac.New(sha1.New, []byte(key)).Sum([]byte(value)), value)
}

// Returns a string if HMAC signature is valid.
func ValidateSignedString(value, key string) (string, error) {
	parts := strings.SplitN(value, "----", 2)
	if parts[0] == fmt.Sprintf("%x", hmac.New(sha1.New, []byte(key)).Sum([]byte(parts[1]))) {
		return parts[1], nil
	}
	return "", errors.New("data is tampered")
}

// }}}
