//go:build ignore

package main

import (
	"bytes"
	"os"
	"os/exec"

	"golang.org/x/crypto/ssh"
)

func keyscan(hostname string) []byte {
	var out bytes.Buffer

	cmd := exec.Command("ssh-keyscan", hostname)
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		panic(err)
	}
	return out.Bytes()
}

func main() {
	if len(os.Args) < 2 {
		panic("no hostname specified")
	}

	key, _, _, _, err := ssh.ParseAuthorizedKey(keyscan(os.Args[1]))
	if err != nil {
		panic(err)
	}

	if err := os.WriteFile("host_key", key.Marshal(), 0644); err != nil {
		panic(err)
	}
}
