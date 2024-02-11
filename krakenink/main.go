package main

import (
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
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
	//go:embed template.html
	tFile embed.FS
	// dbpath = filepath.Join("/", "mnt", "onboard", ".kobo", "KoboReader.sqlite")
	dbpath = "/run/media/speedking/KOBOeReader/.kobo/KoboReader.sqlite"
)

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

func writeHTMLs(data map[string][]Bookmark) {
	t, err := template.ParseFS(tFile, "template.html")
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup
	for title, bookmarks := range data {
		wg.Add(1)
		go func() {
			defer wg.Done()

			f, err := os.Create(title + ".html")
			if err != nil {
				log.Println(err)
				return
			}
			defer f.Close()

			if err := t.Execute(f, TempData{Title: title, Bookmarks: bookmarks}); err != nil {
				log.Println(err)
			}
		}()
	}

	wg.Wait()
}

func main() {
	data, err := readBookmarks()
	if err != nil {
		log.Fatal(err)
	}
	writeHTMLs(data)
}
