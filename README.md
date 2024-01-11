# TortugaSync
My own custom cloud system for Kobo eBooks.

To use Tortuga Sync you need to have:
- A new pair of *passwordless* ssh keys: `ssh-keygen -t ed25519 -f tortuga_key`
- A *host_key* file containing the server public key: `go run hostkey.go myhostname` (requires *ssh-keyscan*, replace 'myhostname' with your actual hostname)
- A host_address file containing the address and port to use for the server (can be an IP) (www.example.com:22), make sure to *not* include a new line at the end of the file.
