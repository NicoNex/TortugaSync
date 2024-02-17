package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	var port string

	flag.StringVar(&port, "-p", ":8085", "Specify the port to use.")
	flag.Parse()

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	path := filepath.Join(home, "bookmarks")
	http.Handle("/", http.FileServer(http.Dir(path)))
	log.Println(http.ListenAndServe(port, nil))
}
