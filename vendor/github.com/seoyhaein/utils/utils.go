package utils

import (
	"fmt"
	"os"
	"strings"

	jsoniter "github.com/json-iterator/go"
)

// 추후에는 내용이 많아지면 분리하자.
var (
	pTrue = true
	PTrue = &pTrue

	pFalse = false
	PFalse = &pFalse
)

//IsEmptyString true if string is empty, false otherwise
func IsEmptyString(s string) bool {
	r := len(strings.TrimSpace(s))

	if r == 0 {
		return true
	}
	return false
}

var json = jsoniter.ConfigCompatibleWithStandardLibrary

// DeepCopy does a deep copy of a structure
// Error checking of parameters delegated to json engine
var DeepCopy = func(dst interface{}, src interface{}) error {
	payload, err := json.Marshal(src)
	if err != nil {
		return err
	}

	err = json.Unmarshal(payload, dst)
	if err != nil {
		return err
	}
	return nil
}

// FileExists true if the file exists, false if the file does not exist
// If the file exists, the FileInfo of the file is returned.
func FileExists(path string) (bool, os.FileInfo, error) {
	var (
		fileInfo os.FileInfo
		err      error
	)
	if IsEmptyString(path) {
		return false, nil, fmt.Errorf("path is emtpy")
	}
	if fileInfo, err = os.Stat(path); os.IsNotExist(err) {
		return false, nil, fmt.Errorf("the file does not exist")
	}
	return true, fileInfo, nil
}

func Truncate(path string) error {

	file, err := os.OpenFile(path, os.O_RDWR, 0666)
	defer file.Close()

	if err != nil {
		return err
	}

	err = file.Truncate(0)
	if err != nil {
		return err
	}
	_, err = file.Seek(0, 0)
	if err != nil {
		return err
	}

	return nil
}
