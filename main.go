package tortugasync

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Bay struct {
	*sftp.Client
}

func Connect(url string, key, hostKey []byte) (Bay, error) {
	signer, err := ssh.ParsePrivateKey(key)
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

	return Bay{Client: client}, nil
}

// Metadata returns a map of MD5 hashes as keys and file paths as values read from the server.
func (b Bay) Metadata(path string) (Cache, error) {
	meta, err := b.Open(path)
	if err != nil {
		return nil, fmt.Errorf("b.metadata: b.Open: %w", err)
	}
	defer meta.Close()

	ret := make(Cache)
	cnt, err := io.ReadAll(meta)
	if err != nil {
		return nil, fmt.Errorf("b.metadata: io.ReadAll: %w", err)
	}

	if err := json.Unmarshal(cnt, &ret); err != nil {
		return nil, fmt.Errorf("b.metadata: json.Unmarshal: %w", err)
	}
	return ret, nil
}

// Fetch downloads a book from tortuga@{remote}:{remotePath} to localPath and
// returns its just calculated MD5SUM and an error if any.
func (b Bay) Fetch(localPath, remotePath string) ([]byte, error) {
	rbook, err := b.Open(remotePath)
	if err != nil {
		return nil, fmt.Errorf("b.fetch: b.Open: %w", err)
	}
	defer rbook.Close()

	lbook, err := os.Create(localPath)
	if err != nil {
		return nil, fmt.Errorf("b.fetch: os.Create: %w", err)
	}
	defer lbook.Close()

	// Write the book locally.
	if _, err := io.Copy(lbook, rbook); err != nil {
		return nil, fmt.Errorf("b.fetch: io.Copy: %w", err)
	}
	lbook.Seek(0, 0)

	// Calculate MD5 of the downloaded book.
	hash := md5.New()
	if _, err := io.Copy(hash, lbook); err != nil {
		return nil, fmt.Errorf("b.fetch: io.Copy: %w", err)
	}

	return hash.Sum(nil), nil
}

func (b Bay) Upload(localPath, remotePath string) error {
	rfile, err := b.OpenFile(remotePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC)
	if err != nil {
		return err
	}
	defer rfile.Close()

	lfile, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer lfile.Close()

	_, err = io.Copy(rfile, lfile)
	return err
}
