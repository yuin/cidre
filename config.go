package cidre

import (
	"errors"
	"fmt"
	"io/ioutil"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ConfigContainer represents a section in configuration files.
type ConfigContainer map[string]map[string]interface{}

type ConfigMapping struct {
	Section string
	Struct  interface{}
}

// Attempts to read and parse the given filepath, Mapping sections to the given object.
// Configuration file format is simplified ini format.
//
// Example:
//    [section1]
//    ; string value: no multiline string support
//    Key1 = String value
//    ; bool value
//    Key2 = true
//    ; int value
//    Key3 = 9999
//    ; float value
//    Key3 = 99.99
//    ; time.Duration value
//    Key3 = 180s
//
//    [section2]
//    ; blah-blah-blah
func ParseIniFile(filepath string, mappings ...ConfigMapping) (ConfigContainer, error) {
	cbytes, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	result := ConfigContainer(make(map[string]map[string]interface{}))
	var current map[string]interface{}
	cstrings := string(cbytes)
	patterns := []*regexp.Regexp{
		/* 0:spaces,comments */ regexp.MustCompile(`^(\s*|\s*[#;].*)$`),
		/* 1:secsions */ regexp.MustCompile(`^\s*\[([^\]]+)\]\s*$`),
		/* 2:bool */ regexp.MustCompile(`^\s*([^=]+)=\s*(true|false)\s*$`),
		/* 3:int */ regexp.MustCompile(`^\s*([^=]+)=\s*(\-?\d+)\s*$`),
		/* 4:float */ regexp.MustCompile(`^\s*([^=]+)=\s*(\-?\d+(\.\d+)?)\s*$`),
		/* 5:time.Duration */ regexp.MustCompile(`^\s*([^=]+)=\s*(\-?\d+(\.\d+)?(ns|us|ms|s|m|h))\s*$`),
		/* 6:string */ regexp.MustCompile(`^\s*([^=]+)=\s*(.*)\s*$`),
	}
	sr := strings.NewReplacer("\\t", "\u0009", "\\n", "\u000A", "\\r", "\u000D")
	for i, line := range strings.Split(cstrings, "\n") {
		failed := true
		for j, pattern := range patterns {
			if matched := pattern.FindStringSubmatch(line); len(matched) > 0 {
				failed = false
				v1 := strings.TrimSpace(matched[1])
				switch j {
				case 1:
					result[v1] = make(map[string]interface{})
					current = result[v1]
				case 2:
					value, _ := strconv.ParseBool(matched[2])
					current[v1] = value
				case 3:
					value, _ := strconv.ParseInt(matched[2], 10, 64)
					current[v1] = value
				case 4:
					value, _ := strconv.ParseFloat(matched[2], 64)
					current[v1] = value
				case 5:
					value, _ := time.ParseDuration(matched[2])
					current[v1] = value
				case 6:
					current[v1] = sr.Replace(matched[2])
				}
				break
			}
		}
		if failed {
			return nil, errors.New(fmt.Sprintf("syntax error: file %v, line %v", filepath, i+1))
		}
	}
	for _, mapping := range mappings {
		result.Mapping(mapping.Section, mapping.Struct)
	}
	return result, nil
}

func (cc ConfigContainer) Mapping(section string, sdata interface{}) {
	mdata := cc[section]
	vt := reflect.ValueOf(sdata).Elem()
	tt := reflect.TypeOf(sdata).Elem()
	for i := 0; i < vt.NumField(); i += 1 {
		if value, ok := mdata[tt.Field(i).Name]; ok {
			switch value.(type) {
			case int64:
				vt.Field(i).SetInt(value.(int64))
			case float64:
				vt.Field(i).SetFloat(value.(float64))
			default:
				vt.Field(i).Set(reflect.ValueOf(value))
			}
		}
	}
}
