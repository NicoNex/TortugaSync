package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NicoNex/echotron/v3"
	"github.com/pgaskin/kepubify/v4/kepub"

	_ "embed"
)

type bot struct {
	chatID int64
	meta   map[string]string
	echotron.API
}

const (
	intro  = "Ahoy there, me hearty! I be Vessellotron, the swashbucklin’ robot ship who’ll help ye transport yer book barrels to yer Kobo!"
	noBook = "Arr, there be no barrels to be had, matey!"
)

var (
	//go:embed token
	token    string
	home     string
	metaPath string

	supportedExts = []string{
		".epub",
		".mobi",
		".pdf",
		".jpeg",
		".jpg",
		".gif",
		".png",
		".bmp",
		".tiff",
		".txt",
		".html",
		".rtf",
		".cbz",
		".cbr",
	}
)

func newBot(chatID int64) echotron.Bot {
	return &bot{
		chatID: chatID,
		meta:   loadMeta(metaPath),
		API:    echotron.NewAPI(token),
	}
}

func (b *bot) Update(update *echotron.Update) {
	switch msg := update.Message.Text; msg {
	case "/start":
		if _, err := b.SendMessage(intro, b.chatID, nil); err != nil {
			log.Println("Update", "b.SendMessage", err)
		}

	default:
		if update.Message.Document == nil {
			_, err := b.SendMessage(noBook, b.chatID, nil)
			if err != nil {
				log.Println("Update", "b.SendMessage", err)
			}
			return
		}
		b.saveEbook(update.Message.Document)
		b.SendMessage("Thanks matey!", b.chatID, nil)
	}
}

func (b bot) saveEbook(doc *echotron.Document) {
	defer b.updateMeta()

	ext := filepath.Ext(doc.FileName)

	if !isAllowedExt(ext) {
		b.SendMessage("Unsupported extension", b.chatID, nil)
		return
	}

	res, err := b.GetFile(doc.FileID)
	if err != nil {
		log.Println("b.saveEbook", "b.GetFile", err)
		b.SendMessage("An error occurred while downloading the eBook.", b.chatID, nil)
		return
	}

	data, err := b.DownloadFile(res.Result.FilePath)
	if err != nil {
		log.Println("b.saveEbook", "b.DownloadFile", err)
		b.SendMessage("An error occurred while downloading the eBook.", b.chatID, nil)
		return
	}

	if !strings.Contains(doc.FileName, ".kepub") && ext == ".epub" {
		if err := b.kepubify(doc.FileName, data); err != nil {
			log.Println(err)
			b.SendMessage("An error occurred while converting to kepub, normal epub will be saved.", b.chatID, nil)
			goto saveOnDisK
		}
		return
	}

saveOnDisK:
	fname := filepath.Join(home, doc.FileName)
	if err := os.WriteFile(fname, data, 0644); err != nil {
		b.SendMessage("An error occurred while saving the file.", b.chatID, nil)
		log.Println("b.saveEbook", "os.WriteFile", err)
	}
	hash := md5.New()

	if _, err := io.Copy(hash, bytes.NewReader(data)); err != nil {
		b.SendMessage("An error occurred while hashing the book", b.chatID, nil)
		log.Println("b.saveEbook", "io.Copy", err)
		return
	}
	b.meta[hex.EncodeToString(hash.Sum(nil))] = fname
}

func (b *bot) kepubify(fname string, data []byte) error {
	fpath := filepath.Join(home, kepubName(fname))
	f, err := os.Create(fpath)
	if err != nil {
		return fmt.Errorf("b.kepubify: os.Create:%w", err)
	}
	defer f.Close()

	bfs, tmpname, err := fsFromBytes(fname, data)
	if err != nil {
		return fmt.Errorf("b.kepubify: fsFromBytes: %w", err)
	}
	defer os.Remove(tmpname)

	conv := kepub.NewConverterWithOptions(kepub.ConverterOptionSmartypants())
	if err := conv.Convert(context.Background(), f, bfs); err != nil {
		return fmt.Errorf("b.kepubify: conv.Convert: %w", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("b.kepubify", "f.Seek", err)
	}

	hash := md5.New()
	if _, err := io.Copy(hash, f); err != nil {
		return fmt.Errorf("b.kepubify", "io.Copy", err)
	}
	b.meta[hex.EncodeToString(hash.Sum(nil))] = fpath

	return nil
}

func (b bot) updateMeta() {
	j, err := json.MarshalIndent(b.meta, "", "  ")
	if err != nil {
		log.Println("b.updateMeta", "json.MarshalIndent", err)
	}
	if err := os.WriteFile(metaPath, j, 0644); err != nil {
		log.Println("b.updateMeta", "os.WriteFile", err)
	}
}

func fsFromBytes(fname string, b []byte) (fs.FS, string, error) {
	tmpf, err := os.CreateTemp("", fname)
	if err != nil {
		return nil, "", err
	}

	if _, err := tmpf.Write(b); err != nil {
		return nil, "", err
	}
	if _, err := tmpf.Seek(0, io.SeekStart); err != nil {
		return nil, "", err
	}
	reader, err := zip.NewReader(tmpf, int64(len(b)))
	return reader, tmpf.Name(), err
}

func isAllowedExt(ext string) bool {
	ext = strings.ToLower(ext)
	for _, e := range supportedExts {
		if e == ext {
			return true
		}
	}
	return false
}

func kepubName(name string) string {
	name = filepath.Base(name)
	ext := filepath.Ext(name)
	return name[:len(name)-len(ext)] + ".kepub.epub"
}

func loadMeta(path string) (m map[string]string) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]string)
		}
		log.Fatalln("loadMeta", "os.ReadFile", err)
	}

	if err := json.Unmarshal(b, &m); err != nil {
		log.Fatalln("loadMeta", "json.Unmarshal", err)
	}
	return
}

func main() {
	var (
		dsp  = echotron.NewDispatcher(token, newBot)
		opts = echotron.UpdateOptions{
			Timeout: 120,
			AllowedUpdates: []echotron.UpdateType{
				echotron.MessageUpdate,
			},
		}
	)

	for {
		log.Println(dsp.PollOptions(false, opts))
		time.Sleep(5 * time.Second)
	}
}

func init() {
	h, err := os.UserHomeDir()
	if err != nil {
		log.Fatalln(err)
	}
	home = h
	metaPath = filepath.Join(home, "metadata.json")
}
