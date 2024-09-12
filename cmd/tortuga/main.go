package main

import (
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
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

type Book struct {
	ID        string
	Title     string
	Author    string
	Bookmarks []Bookmark
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

	serverHome = filepath.Join("/", "home", "tortuga")
	bmpath     = filepath.Join(serverHome, ".kraken")
	koboHome   = filepath.Join("/", "mnt", "onboard")
	notespath  = filepath.Join("/", "mnt", "onboard", ".kraken_notes")
	dbpath     = filepath.Join("/", "mnt", "onboard", ".kobo", "KoboReader.sqlite")
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

func readBookmarks() (map[string]*Book, error) {
	db, err := sql.Open("sqlite", dbpath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(
		`SELECT
		    Bookmark.VolumeID,
		    Bookmark.Text,
		    Bookmark.Annotation,
		    content.BookTitle,
			content.Attribution
		FROM
		    Bookmark
		JOIN
		    content
		ON
		    Bookmark.VolumeID = content.BookID;`,
	)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// var data = make(map[string][]Bookmark)
	var data = make(map[string]*Book)
	for rows.Next() {
		var (
			ID     string
			text   sql.NullString
			note   sql.NullString
			title  sql.NullString
			author sql.NullString
		)

		if err := rows.Scan(&ID, &text, &note, &title, &author); err != nil {
			fmt.Println(err)
			continue
		}

		if author.Valid {
			fmt.Println(author.String)
		}
		book, ok := data[ID]
		if !ok {
			book = &Book{ID: ID, Title: title.String}
			data[ID] = book
		}
		if author.Valid {
			book.Author = author.String
		}
		book.Bookmarks = append(
			book.Bookmarks,
			Bookmark{Text: text.String, Note: note.String},
		)
	}

	return deduplicate(data), rows.Err()
}

func deduplicate(data map[string]*Book) map[string]*Book {
	var ret = make(map[string]*Book)

	for id, book := range data {
		b := &Book{
			ID:     book.ID,
			Title:  book.Title,
			Author: book.Author,
		}

		bmtab := make(map[string]bool)
		for _, bm := range book.Bookmarks {
			if _, ok := bmtab[bm.Text]; !ok {
				b.Bookmarks = append(b.Bookmarks, bm)
				bmtab[bm.Text] = true
			}
		}

		ret[id] = b
	}

	return ret
}

func normalise(ID string, book Book) TempData {
	ret := TempData{
		Title:     book.Title,
		Bookmarks: book.Bookmarks,
	}

	if ret.Title == "" {
		ret.Title = ID
	}
	if book.Author != "" {
		ret.Title += " - " + book.Author
	}
	return ret
}

func uploadBookmarks(bay ts.Bay, data map[string]*Book) (e error) {
	t, err := template.ParseFS(tFile, "template.html")
	if err != nil {
		return err
	}

	pchan := make(chan string, len(data))
	done := make(chan bool, 1)
	go func() {
		for path := range pchan {
			rpath := filepath.Join(bmpath, filepath.Base(path))
			if err := bay.Upload(path, rpath); err != nil {
				fmt.Println("uploadBookmarks", "bay.Upload", err)
			}
		}
		done <- true
	}()

	var wg sync.WaitGroup
	for ID, book := range data {
		ID = filepath.Base(ID)

		bmpath := filepath.Join(notespath, ID+".html")
		wg.Add(1)
		go func() {
			defer wg.Done()

			f, err := os.Create(bmpath)
			if err != nil {
				e = errors.Join(e, err)
				return
			}
			defer f.Close()

			if err := t.Execute(f, normalise(ID, *book)); err != nil {
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
	flag.BoolVar(&isKraken, "b", false, "Upload bookmarks to the server")
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

func init() {
	if _, err := os.Stat(notespath); errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(notespath, 0755); err != nil {
			fmt.Println(err)
		}
	}
}
