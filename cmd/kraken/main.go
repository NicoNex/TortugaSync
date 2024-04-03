package main

import (
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"embed"
)

//go:embed template.html
var templHTML embed.FS

func main() {
	var port string

	flag.StringVar(&port, "p", ":8085", "Specify the port to use.")
	flag.Parse()

	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	t, err := template.ParseFS(templHTML, "template.html")
	if err != nil {
		log.Fatal(err)
	}

	path := filepath.Join(home, "bookmarks")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		files, err := os.ReadDir(path)
		if err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if err = t.Execute(w, files); err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	})
	http.Handle("/file/", http.StripPrefix("/file/", http.FileServer(http.Dir(path))))
	log.Println(http.ListenAndServe(port, nil))
}
