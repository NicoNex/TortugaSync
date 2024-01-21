package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"

	_ "embed"
)

var (
	bay Bay

	//go:embed host_address
	hostAddress string
	//go:embed tortuga_key
	tortugaKey []byte
	//go:embed host_key
	hostKey []byte

	koboHome   = filepath.Join("/", "mnt", "onboard")
	serverHome = filepath.Join("/", "home", "tortuga")
	cachePath  = filepath.Join(koboHome, ".cache", "tortuga-sync")
)

type Bay struct {
	ccpath string
	cache  map[string]string
	*sftp.Client
}

func Connect(url string) (Bay, error) {
	signer, err := ssh.ParsePrivateKey(tortugaKey)
	if err != nil {
		return Bay{}, fmt.Errorf("Connect: ssh.ParsePrivateKey: %w", err)
	}

	clientCfg := &ssh.ClientConfig{
		User: "tortuga",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		Timeout: 5 * time.Second,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			if !bytes.Equal(key.Marshal(), hostKey) {
				return errors.New("HostKeyCallback: invalid host key")
			}
			return nil
		},
	}

	// Connect to the remote server.
	conn, err := ssh.Dial("tcp", url, clientCfg)
	if err != nil {
		return Bay{}, fmt.Errorf("Connect: ssh.Dial: %w", err)
	}

	// Open an SFTP session over the SSH connection.
	client, err := sftp.NewClient(conn)
	if err != nil {
		return Bay{}, fmt.Errorf("Connect: %w", err)
	}

	ccpath := filepath.Join(koboHome, "tortuga.json")
	cc, err := cache(ccpath)
	if err != nil {
		return Bay{}, fmt.Errorf("Connect: cache: %w", err)
	}

	return Bay{ccpath: ccpath, cache: cc, Client: client}, nil
}

// metadata returns a map of MD5 hashes as keys and file paths as values.
func (b Bay) metadata() (map[string]string, error) {
	meta, err := b.Open(filepath.Join(serverHome, "metadata.json"))
	if err != nil {
		return nil, fmt.Errorf("b.metadata: b.Open: %w", err)
	}
	defer meta.Close()

	ret := make(map[string]string)
	cnt, err := io.ReadAll(meta)
	if err != nil {
		return nil, fmt.Errorf("b.metadata: io.ReadAll: %w", err)
	}

	if err := json.Unmarshal(cnt, &ret); err != nil {
		return nil, fmt.Errorf("b.metadata: json.Unmarshal: %w", err)
	}
	return ret, nil
}

func (b Bay) fetch(path string) error {
	rbook, err := b.Open(path)
	if err != nil {
		return fmt.Errorf("b.fetch: b.Open: %w", err)
	}
	defer rbook.Close()

	lpath := filepath.Join(koboHome, filepath.Base(path))
	lbook, err := os.Create(lpath)
	if err != nil {
		return fmt.Errorf("b.fetch: os.Create: %w", err)
	}
	defer lbook.Close()

	// Write the book locally.
	if _, err := io.Copy(lbook, rbook); err != nil {
		return fmt.Errorf("b.fetch: io.Copy: %w", err)
	}
	lbook.Seek(0, 0)

	// Calculate MD5 of the downloaded book.
	hash := md5.New()
	if _, err := io.Copy(hash, lbook); err != nil {
		return fmt.Errorf("b.fetch: io.Copy: %w", err)
	}

	b.cache[hex.EncodeToString(hash.Sum(nil))] = lpath
	return nil
}

func (b Bay) ImportAll() (err error) {
	var g = new(errgroup.Group)

	meta, err := b.metadata()
	if err != nil {
		return err
	}

	for _, path := range filter(meta, b.cache) {
		path := path
		g.Go(func() error { return b.fetch(path) })
	}
	if err := g.Wait(); err != nil {
		err = fmt.Errorf("b.ImportAll: %w", err)
	}
	return
}

func (bay Bay) updateCache() error {
	b, err := json.MarshalIndent(bay.cache, "", "  ")
	if err != nil {
		return fmt.Errorf("bay.updateCache: json.MarshalIndent: %w", err)
	}
	return os.WriteFile(bay.ccpath, b, 0644)
}

func (b Bay) Close() (err error) {
	return errors.Join(
		b.Client.Close(),
		b.updateCache(),
	)
}

// filter subtracts the hashes in b from a.
// This is used to evaluate what has to be fetched from the server.
// Since the file hash in the local cache is computed locally this mechanism
// intrinsically makes sure that if the file is corrupted it will be re downloaded
// next time.
func filter(a, b map[string]string) map[string]string {
	for hash := range b {
		delete(a, hash)
	}
	return a
}

func cache(path string) (cc map[string]string, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]string), nil
		}
		return nil, err
	}
	err = json.Unmarshal(b, &cc)
	return
}

func main() {
	bay, err := Connect(hostAddress)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer bay.Close()

	if err := bay.ImportAll(); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("All done!")
}
