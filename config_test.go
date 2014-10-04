package cidre

import (
    "path/filepath"
	"testing"
    "runtime"
    "time"
)

type configTest1Struct struct {
  ConfInt int
  ConfFloat float64
  ConfDuration time.Duration
  ConfString string
}

type configTest2Struct struct {
  ConfInt int
  ConfFloat float64
  ConfDuration time.Duration
  ConfString int
}

func TestConfig(t *testing.T){
  _, file, _, _ := runtime.Caller(0)
  directory := filepath.Dir(file)
  confFile := filepath.Join(directory, "testing", "test1.ini")
  conf1 := &configTest1Struct{10, 10.0, 10, "0"}
  ParseIniFile(confFile, ConfigMapping{"yourconfig1", conf1})
  errorIfNotEqual(t, 1, conf1.ConfInt)
  errorIfNotEqual(t, -3.2, conf1.ConfFloat)
  errorIfNotEqual(t, 10 * time.Second, conf1.ConfDuration)
  errorIfNotEqual(t, "foobar", conf1.ConfString)

  ParseIniFile(confFile, ConfigMapping{"yourconfig2", conf1})
  errorIfNotEqual(t, 2, conf1.ConfInt)
  errorIfNotEqual(t, -3.2, conf1.ConfFloat)
  errorIfNotEqual(t, 10 * time.Second, conf1.ConfDuration)
  errorIfNotEqual(t, "foobar", conf1.ConfString)

  func(){
    defer func(){
      if recv := recover(); recv == nil {
        t.Error("should cause panic when type missmatch")
      }
    }()
    conf1 := &configTest2Struct{10, 10.0, 10, 0}
    ParseIniFile(confFile, ConfigMapping{"yourconfig1", conf1})
  }()
}
