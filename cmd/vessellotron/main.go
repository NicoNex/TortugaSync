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

type empty struct{}

func (e empty) Update(_ *echotron.Update) {}

const (
	NicoNex = 41876271

	intro  = "Ahoy there, me hearty! I be Vessellotron, the swashbucklin’ robot ship who’ll help ye transport yer book barrels to yer Kobo!"
	noBook = "Arr, there be no barrels to be had, matey!"
)

var (
	//go:embed token
	token    string
	home     string
	metaPath string
	dsp      *echotron.Dispatcher

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

	escapeMD = strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	).Replace

	mdopt = &echotron.MessageOptions{ParseMode: echotron.MarkdownV2}
)

func newBot(chatID int64) echotron.Bot {
	if chatID != NicoNex {
		echotron.NewAPI(token).SendMessage("You're not the capitain!", chatID, nil)
		go func() {
			time.Sleep(3 * time.Second)
			dsp.DelSession(chatID)
		}()
		return &empty{}
	}

	return &bot{
		chatID: chatID,
		meta:   loadMeta(metaPath),
		API:    echotron.NewAPI(token),
	}
}

func (b *bot) Update(update *echotron.Update) {
	switch msg := update.Message.Text; {
	case msg == "/start":
		if _, err := b.SendMessage(intro, b.chatID, nil); err != nil {
			log.Println("Update", "b.SendMessage", err)
		}

	case msg == "/refresh":
		b.refreshMeta()
		b.sendMeta()

	case msg == "/metadata":
		b.sendMeta()

	case strings.HasPrefix(msg, "/delete"):
		toks := strings.Split(msg, " ")
		if len(toks) < 2 {
			_, err := b.SendMessage("No hash provided", b.chatID, nil)
			if err != nil {
				log.Println("b.Update", "b.SendMessage", err)
			}
			return
		}
		b.delEbook(toks[1])

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

func (b bot) delEbook(h string) {
	defer b.updateMeta()

	if p, ok := b.meta[h]; ok {
		if err := os.Remove(p); err != nil {
			log.Println("b.Update", "os.Remove", err)
		}
		delete(b.meta, h)
		b.SendMessage("ok", b.chatID, nil)
	} else {
		_, err := b.SendMessage("Unknown hash", b.chatID, nil)
		if err != nil {
			log.Println("b.Update", "b.SendMessage", err)
		}
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
		return fmt.Errorf("b.kepubify: f.Seek: %w", err)
	}

	hash := md5.New()
	if _, err := io.Copy(hash, f); err != nil {
		return fmt.Errorf("b.kepubify: io.Copy: %w", err)
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

func (b bot) refreshMeta() {
	for h, p := range b.meta {
		sum, err := md5sum(p)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				delete(b.meta, h)
			} else {
				log.Println("b.refreshMeta", "md5sum", err)
			}
			continue
		}

		if sum != h {
			delete(b.meta, h)
			b.meta[sum] = p
		}
	}
	b.updateMeta()
}

func (b bot) sendMeta() {
	var (
		cnt int
		buf strings.Builder
	)

	for h, p := range b.meta {
		if cnt >= 10 {
			_, err := b.SendMessage(buf.String(), b.chatID, mdopt)
			if err != nil {
				log.Println("b.sendMeta", "b.SendMessage", err)
			}
			buf.Reset()
			cnt = 0
		}

		buf.WriteString(fmt.Sprintf("*%s*\n`%s`\n\n", escapeMD(filepath.Base(p)), h))
		cnt++
	}

	_, err := b.SendMessage(buf.String(), b.chatID, mdopt)
	if err != nil {
		log.Println("b.sendMeta", "b.SendMessage", err)
	}
}

func md5sum(path string) (string, error) {
	var hash = md5.New()

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
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
	opts := echotron.UpdateOptions{
		Timeout: 120,
		AllowedUpdates: []echotron.UpdateType{
			echotron.MessageUpdate,
		},
	}

	dsp = echotron.NewDispatcher(token, newBot)
	for {
		log.Println("dsp.PollOptions", dsp.PollOptions(false, opts))
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

	echotron.NewAPI(token).SetMyCommands(
		nil,
		echotron.BotCommand{Command: "/start", Description: "Start the chat with the bot"},
		echotron.BotCommand{Command: "/refresh", Description: "Refresh books' metadata"},
		echotron.BotCommand{Command: "/metadata", Description: "Sends the eBooks' metadata"},
		echotron.BotCommand{Command: "/delete", Description: "Deletes an eBook"},
	)
}
