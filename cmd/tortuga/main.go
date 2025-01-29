package main

import (
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"text/template"

	ts "github.com/NicoNex/tortugasync"
	_ "modernc.org/sqlite"

	_ "embed"
)

type Bookmark struct {
	Text string `json:"text"`
	Note string `json:"note,omitempty"`
}

type Book struct {
	ID        string     `json:"-"`
	Title     string     `json:"title"`
	Author    string     `json:"author"`
	Bookmarks []Bookmark `json:"bookmarks"`
}

var (
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

	sre = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]`)
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

func uploadBookmarks(bay ts.Bay, localPaths <-chan string) <-chan struct{} {
	var done = make(chan struct{}, 1)

	go func() {
		defer close(done)

		for path := range localPaths {
			rpath := filepath.Join(bmpath, filepath.Base(path))
			if err := bay.Upload(path, rpath); err != nil {
				fmt.Println("uploadBookmarks", "bay.Upload", err)
			}
		}
		done <- struct{}{}
	}()
	return done
}

func generateBookmarks(data map[string]*Book) <-chan string {
	var paths = make(chan string)

	go func() {
		defer close(paths)

		t, err := template.ParseFS(tFile, "template.html")
		if err != nil {
			fmt.Println("generateBookmarks", "template.ParseFS", err)
			return
		}

		var wg sync.WaitGroup
		for id, book := range data {
			wg.Add(1)
			go func() {
				defer wg.Done()
				path := filepath.Join(
					notespath,
					notename(id, book.Title, book.Author),
				)

				f, err := os.Create(path)
				if err != nil {
					fmt.Println("generateBookmarks", "os.Create", err)
					return
				}
				defer f.Close()

				if err := t.Execute(f, book); err != nil {
					fmt.Println("generateBookmarks", "t.Execute", err)
					return
				}
				paths <- path
			}()
		}
		wg.Wait()
	}()

	return paths
}

func generateBookmarksJSON(data map[string]*Book) <-chan string {
	var paths = make(chan string)

	go func() {
		defer close(paths)

		var wg sync.WaitGroup
		for id, book := range data {
			wg.Add(1)
			go func() {
				defer wg.Done()

				b, err := json.Marshal(book)
				if err != nil {
					fmt.Println("generateBookmarksJSON", "json.Marshal", err)
					return
				}

				path := filepath.Join(
					notespath,
					jsonname(id, book.Title, book.Author),
				)
				if err := os.WriteFile(path, b, os.ModePerm); err != nil {
					fmt.Println("generateBookmarksJSON", "os.WriteFile", err)
					return
				}
				paths <- path
			}()
		}
		wg.Wait()
	}()

	return paths
}

func notename(ID, title, author string) string {
	if title == "" {
		return sre.ReplaceAllString(filepath.Base(ID), "") + ".html"
	}

	var name = title
	if author != "" {
		name += " - " + author
	}
	if len(name) > 250 {
		name = name[:250]
	}
	name = sre.ReplaceAllString(name, "")

	return name + ".html"
}

func jsonname(ID, title, author string) string {
	if title == "" {
		return sre.ReplaceAllString(filepath.Base(ID), "") + ".json"
	}

	var name = title
	if author != "" {
		name += " - " + author
	}
	if len(name) > 250 {
		name = name[:250]
	}
	name = sre.ReplaceAllString(name, "")

	return name + ".json"
}

func queryData(db *sql.DB) (map[string]*Book, error) {
	var data = make(map[string]*Book)

	rows, e := db.Query(
		`SELECT
		    Bookmark.VolumeID,
		    Bookmark.Text,
		    Bookmark.Annotation,
		    (
		        SELECT BookTitle
		        FROM content
		        WHERE content.BookID = Bookmark.VolumeID
		        LIMIT 1
		    ) AS BookTitle,
		    (
		        SELECT Attribution
		        FROM content
		        WHERE content.ContentID = Bookmark.VolumeID
		          AND content.Attribution IS NOT NULL
		          AND content.Attribution != ''
		        LIMIT 1
		    ) AS Author
		FROM
		    Bookmark
		WHERE
			Bookmark.Text IS NOT NULL AND Bookmark.Text != '';`,
	)
	if e != nil {
		return data, fmt.Errorf("queryData db.Query %w", e)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id     string
			text   sql.NullString
			note   sql.NullString
			title  sql.NullString
			author sql.NullString
		)

		if err := rows.Scan(&id, &text, &note, &title, &author); err != nil {
			e = errors.Join(e, fmt.Errorf("queryData rows.Scan %w", err))
			continue
		}

		book, ok := data[id]
		if !ok {
			book = &Book{ID: id, Title: title.String, Author: author.String}
			data[id] = book
		}
		book.Bookmarks = append(
			book.Bookmarks,
			Bookmark{Text: text.String, Note: note.String},
		)
	}
	return data, errors.Join(e, rows.Err())
}

func readBookmarks() (map[string]*Book, error) {
	db, err := sql.Open("sqlite", dbpath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return queryData(db)
}

func main() {
	var (
		isKraken     bool
		isKrakenJson bool
	)

	flag.BoolVar(&isKraken, "b", false, "Upload bookmarks to the server")
	flag.BoolVar(&isKrakenJson, "bm-json", false, "Upload bookmarks to the server in JSON format")
	flag.Parse()

	bay, err := ts.Connect(hostAddress, tortugaKey, hostKey)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer bay.Close()

	switch {
	case isKrakenJson:
		bms, err := readBookmarks()
		if err != nil {
			fmt.Println(err)
			return
		}
		<-uploadBookmarks(bay, generateBookmarksJSON(bms))

	case isKraken:
		bms, err := readBookmarks()
		if err != nil {
			fmt.Println(err)
			return
		}
		<-uploadBookmarks(bay, generateBookmarks(bms))

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
