package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/jrick/ss/keyfile"
	"github.com/jrick/ss/stream"
	"golang.org/x/crypto/ssh/terminal"
)

func ssAppDir() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err

	}
	if u.HomeDir == "" {
		return "", fmt.Errorf("user home dir is unknown")

	}
	dir := filepath.Join(u.HomeDir, ".ss")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0700)
		if err != nil {
			return "", err
		}

	}
	return dir, nil
}

func promptPassphrase(prompt string) ([]byte, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		panic(err)

	}
	defer tty.Close()
	_, err = fmt.Fprintf(tty, "%s: ", prompt)
	if err != nil {
		panic(err)

	}
	passphrase, err := terminal.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(tty)
	return passphrase, err
}

func decryptPrivKeyFile(privKeyFile string, pk *[32]byte) error {
	in, err := os.Open(privKeyFile)
	if err != nil {
		return err
	}

	stat, err := in.Stat()
	if err != nil {
		return err
	}

	header, err := stream.ReadHeader(in)
	if err != nil {
		return fmt.Errorf("error reading privKeyFile header: %v", err)
	}

	var key *stream.SymmetricKey

	switch header.Scheme {
	case stream.StreamlinedNTRUPrime4591761Scheme:
		// Read and decrypt secret key
		appdir, err := ssAppDir()
		if err != nil {
			return err
		}
		skFilename := filepath.Join(appdir, "id.secret")
		skFile, err := os.Open(skFilename)
		if err != nil {
			return err
		}
		passphrase, err := promptPassphrase(fmt.Sprintf("Key passphrase for %s", skFilename))
		if err != nil {
			return err
		}
		sk, _, err := keyfile.OpenSecretKey(skFile, passphrase)
		if err != nil {
			return fmt.Errorf("unable to open secret keyfile: %v", err)
		}
		key, err = stream.Decapsulate(header, sk)
		if err != nil {
			return err
		}
	case stream.Argon2idScheme:
		passphrase, err := promptPassphrase("Decryption passphrase")
		if err != nil {
			return err
		}
		key, err = stream.PassphraseKey(header, passphrase)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown header scheme: %v", header.Scheme)
	}

	// Sizing by stat.Size() is overestimating the plaintext size but
	// ensures buf[] won't be resized so we'll be able to securily clear it
	// out without leaving copies around in memory.
	buf := make([]byte, int(stat.Size()))
	out := bytes.NewBuffer(buf[:0])
	err = stream.Decrypt(out, in, header.Bytes, key)
	if err != nil {
		return err
	}
	log.Debugf("Plaintext size: %d", out.Len())

	// Plaintext is in hex, decode from hex into *pk.
	hexPk := bytes.TrimSpace(buf)
	_, err = hex.Decode(pk[:], hexPk)
	zeroBytes(buf)

	return nil
}
