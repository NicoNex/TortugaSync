package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	ts "github.com/NicoNex/tortugasync"

	_ "embed"
)

var (
	//go:embed host_address
	hostAddress string
	//go:embed tortuga_key
	tortugaKey []byte
	//go:generate go run hostkey.go
	//go:embed host_key
	hostKey []byte

	// koboHome   = filepath.Join("/", "mnt", "onboard")
	koboHome   = filepath.Join(".", "/", "test")
	serverHome = filepath.Join("/", "home", "tortuga")
)

func downloadAll(bay ts.Bay) (e error) {
	ccPath := filepath.Join(koboHome, "tortuga.json")
	lcache, err := ts.NewCacheFromFile(ccPath)
	if err != nil {
		return err
	}

	rcache, err := bay.Metadata(filepath.Join(serverHome, "metadata.json"))
	if err != nil {
		return err
	}

	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)
	for _, path := range rcache.Diff(lcache) {
		lpath := filepath.Join(koboHome, filepath.Base(path))
		b, err := bay.Fetch(lpath, path)
		if err != nil {
			e = errors.Join(e, err)
			continue
		}
		lcache[hex.EncodeToString(b)] = lpath
		wg.Add(1)
		go func() {
			mu.Lock()
			lcache.WriteToFile(ccPath)
			mu.Unlock()
			wg.Done()
		}()
	}

	wg.Wait()
	return nil
}

func main() {
	bay, err := ts.Connect(hostAddress, tortugaKey, hostKey)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer bay.Close()

	downloadAll(bay)
	fmt.Println("All done!")
}
