package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/cors"
)

var jpath string

func handleList(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(jpath)
	if err != nil {
		log.Println("handleList", "os.ReadDir", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var jsonFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			jsonFiles = append(jsonFiles, entry.Name())
		}
	}

	w.Header().Set("Content-Type", "application/json")
	b, err := json.Marshal(jsonFiles)
	if err != nil {
		log.Println("handleList", "json.Marshal", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Write(b)
}

func main() {
	var port string

	flag.StringVar(&port, "p", ":8080", "Specify the port to use.")
	flag.Parse()

	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	mux := http.NewServeMux()
	handler := cors.New(cors.Options{
		AllowedOrigins: []string{
			"https://ukraken.dobl.one",
		},
		AllowedMethods: []string{"GET", "OPTIONS"},
		AllowedHeaders: []string{"*"},
	}).Handler(mux)

	mux.HandleFunc("/list", handleList)
	mux.Handle("/json/", http.StripPrefix("/json/", http.FileServer(http.Dir(jpath))))
	for {
		log.Println("main", "http.ListenAndServe", http.ListenAndServe(port, handler))
		time.Sleep(5 * time.Second)
	}
}

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("init", "os.UserHomeDir", err)
	}
	jpath = filepath.Join(home, ".kraken-json")
}
