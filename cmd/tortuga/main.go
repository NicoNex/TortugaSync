package main

import (
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"text/template"

	ts "github.com/NicoNex/tortugasync"
	_ "modernc.org/sqlite"

	_ "embed"
)

type Bookmark struct {
	Text string
	Note string
}

func (b Bookmark) String() string {
	return fmt.Sprintf("%s\n%s", b.Text, b.Note)
}

type TempData struct {
	Title     string
	Bookmarks []Bookmark
}

var (
	isKraken bool

	//go:embed host_address
	hostAddress string
	//go:embed tortuga_key
	tortugaKey []byte
	//go:generate go run hostkey.go
	//go:embed host_key
	hostKey []byte
	//go:embed template.html
	tFile embed.FS

	koboHome   = filepath.Join("/", "mnt", "onboard")
	serverHome = filepath.Join("/", "home", "tortuga")
	// notespath  = filepath.Join("/", "mnt", "onboard", "kraken")
	// dbpath     = filepath.Join("/", "mnt", "onboard", ".kobo", "KoboReader.sqlite")
	notespath = "./notes"
	dbpath    = "/run/media/speedking/KOBOeReader/.kobo/KoboReader.sqlite"
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

func readBookmarks() (map[string][]Bookmark, error) {
	var data = make(map[string][]Bookmark)

	db, err := sql.Open("sqlite", dbpath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`SELECT VolumeID, Text, Annotation FROM Bookmark`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			file string
			bm   Bookmark
		)

		if err := rows.Scan(&file, &bm.Text, &bm.Note); err != nil {
			log.Println(err)
			continue
		}
		file = filepath.Base(file)
		data[file] = append(data[file], bm)
	}

	return data, rows.Err()
}

func uploadBookmarks(bay ts.Bay, data map[string][]Bookmark) (e error) {
	t, err := template.ParseFS(tFile, "template.html")
	if err != nil {
		return err
	}

	pchan := make(chan string, len(data))
	done := make(chan bool, 1)
	defer close(done)
	go func() {
		for path := range pchan {
			rpath := filepath.Join(serverHome, "bookmarks", filepath.Base(path))
			if err := bay.Upload(path, rpath); err != nil {
				fmt.Println(err)
			}
		}
		done <- true
	}()

	var wg sync.WaitGroup
	for title, bookmarks := range data {
		bmpath := filepath.Join(notespath, title+".html")
		wg.Add(1)
		go func() {
			defer wg.Done()

			f, err := os.Create(bmpath)
			if err != nil {
				e = errors.Join(e, err)
				return
			}
			defer f.Close()

			if err := t.Execute(f, TempData{Title: title, Bookmarks: bookmarks}); err != nil {
				e = errors.Join(e, err)
			}
			pchan <- bmpath
		}()
	}

	wg.Wait()
	close(pchan)
	<-done
	close(done)
	return
}

func main() {
	flag.BoolVar(&isKraken, "bookmarks", false, "Upload bookmarks to the server")
	flag.Parse()

	bay, err := ts.Connect(hostAddress, tortugaKey, hostKey)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer bay.Close()

	switch {
	case isKraken:
		bms, err := readBookmarks()
		if err != nil {
			fmt.Println(err)
			return
		}
		if err := uploadBookmarks(bay, bms); err != nil {
			fmt.Println(err)
			return
		}

	default:
		downloadAll(bay)
	}
	fmt.Println("All done!")
}
