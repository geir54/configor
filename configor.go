package configor

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"

	"gopkg.in/yaml.v2"
)

// ENV will return environment
func ENV() string {
	if env := os.Getenv("CONFIGOR_ENV"); env != "" {
		return env
	}
	// return test when running go test
	if isTest, _ := regexp.MatchString("/_test/", os.Args[0]); isTest {
		return "test"
	}
	return "development"
}

func getConfigurationWithENV(file, env string) (string, error) {
	var envFile string
	var extname = path.Ext(file)

	if extname == "" {
		envFile = fmt.Sprintf("%v.%v", file, env)
	} else {
		envFile = fmt.Sprintf("%v.%v%v", strings.TrimSuffix(file, extname), env, extname)
	}

	if fileInfo, err := os.Stat(envFile); err == nil && fileInfo.Mode().IsRegular() {
		return envFile, nil
	}
	return "", fmt.Errorf("failed to find file %v", file)
}

func getConfigurations(files ...string) ([]string, error) {
	var results []string
	env := ENV()
	for i := len(files) - 1; i >= 0; i-- {
		var foundFile bool
		var file = files[i]

		// check configuration
		if fileInfo, err := os.Stat(file); err == nil && fileInfo.Mode().IsRegular() {
			foundFile = true
			results = append(results, file)
		}

		// check env configuration
		if file, err := getConfigurationWithENV(file, env); err == nil {
			foundFile = true
			results = append(results, file)
		}

		// check example configuration
		if !foundFile {
			if example, err := getConfigurationWithENV(file, "example"); err == nil {
				//fmt.Printf("Failed to find configuration %v, using example file %v\n", file, example)
				results = append(results, example)
			} else {
				return nil, errors.New("Failed to find configuration " + file + "\n")
			}
		}
	}
	return results, nil
}

func getPrefix(config interface{}) string {
	if prefix := os.Getenv("CONFIGOR_ENV_PREFIX"); prefix != "" {
		return prefix
	}
	return "configor"
}

// Save will save the configurations to a file name you provide
func Save(config interface{}, filename string) error {
	var js []byte
	var err error

	switch {
	case strings.HasSuffix(filename, ".yaml") || strings.HasSuffix(filename, ".yml"):
		js, err = yaml.Marshal(&config)
	case strings.HasSuffix(filename, ".json"):
		js, err = json.Marshal(&config)
	default:
		return errors.New("Unknown file type")
	}

	if err != nil {
		return nil
	}

	err = ioutil.WriteFile(filename, js, 0600)
	return err
}

// Load will unmarshal configurations to struct from files that you provide
func Load(config interface{}, files ...string) error {
	files, err := getConfigurations(files...)
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := load(config, file); err != nil {
			return err
		}
	}

	if prefix := getPrefix(config); prefix == "-" {
		return processTags(config)
	} else {
		return processTags(config, prefix)
	}
}

func processTags(config interface{}, prefix ...string) error {
	configValue := reflect.Indirect(reflect.ValueOf(config))
	if configValue.Kind() != reflect.Struct {
		return errors.New("invalid config, should be struct")
	}

	configType := configValue.Type()
	for i := 0; i < configType.NumField(); i++ {
		fieldStruct := configType.Field(i)
		field := configValue.Field(i)

		// read configuration from shell env
		var envName = fieldStruct.Tag.Get("env")
		if envName == "" {
			envName = strings.ToUpper(strings.Join(append(prefix, fieldStruct.Name), "_"))
		}

		if envName != "" {
			if value := os.Getenv(envName); value != "" {
				if err := yaml.Unmarshal([]byte(value), field.Addr().Interface()); err != nil {
					return err
				}
			}
		}

		if isBlank := reflect.DeepEqual(field.Interface(), reflect.Zero(field.Type()).Interface()); isBlank {
			// set default configuration if is blank
			if value := fieldStruct.Tag.Get("default"); value != "" {
				if err := yaml.Unmarshal([]byte(value), field.Addr().Interface()); err != nil {
					return err
				}
			} else if fieldStruct.Tag.Get("required") == "true" {
				// set configuration has value if it is required
				return errors.New(fieldStruct.Name + " is required, but blank")
			}
		}

		for field.Kind() == reflect.Ptr {
			field = field.Elem()
		}

		if field.Kind() == reflect.Struct {
			if err := processTags(field.Addr().Interface(), append(prefix, fieldStruct.Name)...); err != nil {
				return err
			}
		}

		if field.Kind() == reflect.Slice {
			var length = field.Len()
			for i := 0; i < length; i++ {
				if reflect.Indirect(field.Index(i)).Kind() == reflect.Struct {
					if err := processTags(field.Index(i).Addr().Interface(), append(prefix, fieldStruct.Name, fmt.Sprintf("%d", i))...); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func load(config interface{}, file string) error {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	switch {
	case strings.HasSuffix(file, ".yaml") || strings.HasSuffix(file, ".yml"):
		return yaml.Unmarshal(data, config)
	case strings.HasSuffix(file, ".toml"):
		return toml.Unmarshal(data, config)
	case strings.HasSuffix(file, ".json"):
		return json.Unmarshal(data, config)
	default:
		if toml.Unmarshal(data, config) != nil {
			if json.Unmarshal(data, config) != nil {
				if yaml.Unmarshal(data, config) != nil {
					return errors.New("failed to decode config")
				}
			}
		}
		return nil
	}
}
