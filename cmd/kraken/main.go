package main

import (
	"embed"
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	//go:embed template.html
	templHTML embed.FS
	//go:embed style.css
	CSS []byte
	//go:embed script.js
	JS []byte

	bpath     string // bookmarks path
	menuTempl *template.Template
)

func files(path string) (files []os.DirEntry, err error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e)
		}
	}
	return
}

func handleMenu(w http.ResponseWriter, r *http.Request) {
	files, err := files(bpath)
	if err != nil {
		log.Println("handleMenu", "files", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	err = menuTempl.Execute(w, files)
	if err != nil {
		log.Println("handleMenu", "menuTempl.Execute", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

func handleCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css")
	if _, err := w.Write(CSS); err != nil {
		log.Println("handleCSS", "w.Write", err)
		return
	}
}

func handleJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	if _, err := w.Write(JS); err != nil {
		log.Println("handleCSS", "w.Write", err)
		return
	}
}

func main() {
	var port string

	flag.StringVar(&port, "p", ":8085", "Specify the port to use.")
	flag.Parse()

	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	http.HandleFunc("/", handleMenu)
	http.HandleFunc("/css", handleCSS)
	http.HandleFunc("/js", handleJS)
	http.Handle("/file/", http.StripPrefix("/file/", http.FileServer(http.Dir(bpath))))

	for {
		log.Println("main", "http.ListenAndServe", http.ListenAndServe(port, nil))
		time.Sleep(5 * time.Second)
	}
}

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("init", "os.UserHomeDir", err)
	}
	bpath = filepath.Join(home, ".kraken")

	menuTempl, err = template.ParseFS(templHTML, "template.html")
	if err != nil {
		log.Fatal("init", "template.ParseFS", err)
	}
}
