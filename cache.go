package tortugasync

import (
	"encoding/json"
	"errors"
	"os"
)

// Cache is a map of MD5 hashes as keys and file paths as values.
// type Cache map[string]string

type Cache map[string]string

// Expects a JSON formatted file path and returns the parsed Cache.
func NewCacheFromFile(path string) (cc Cache, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(Cache), nil
		}
		return nil, err
	}
	err = json.Unmarshal(b, &cc)
	return
}

// Writes the Cache to the given path encoded in JSON format.
func (cc Cache) WriteToFile(path string) error {
	b, err := json.MarshalIndent(cc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

// Diff returns the difference between cc and a.
// This is used to evaluate what has to be fetched from the server.
// Since the file hash in the local cache is computed locally this mechanism
// intrinsically makes sure that if the file is corrupted it will be re downloaded
// next time.
func (cc Cache) Diff(a Cache) Cache {
	var tmp = make(Cache)

	for hash, path := range cc {
		if _, ok := a[hash]; !ok {
			tmp[hash] = path
		}
	}
	return tmp
}
