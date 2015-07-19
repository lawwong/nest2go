package client

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

var ErrServiceClosed = errors.New("service closed")

func LoadPublicKeyFromFile(filename string, pwd string) (pk *rsa.PublicKey, err error) {
	var f *os.File
	f, err = os.Open(filename)
	if err != nil {
		return
	}
	defer f.Close()

	var fs os.FileInfo
	fs, err = f.Stat()
	if err != nil {
		return
	}

	data := make([]byte, fs.Size())
	_, err = f.Read(data)
	if err != nil {
		return
	}

	pblock, _ := pem.Decode(data)
	if pblock == nil {
		err = fmt.Errorf("pem.Decode fail!")
		return
	}

	var der []byte
	if len(pblock.Headers) == 0 || pwd == "" {
		// no header found! used as plain der
		der = pblock.Bytes
	} else {
		der, err = x509.DecryptPEMBlock(pblock, []byte(pwd))
		if err != nil {
			return
		}
	}

	pub, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		// Failed to parse RSA public key
		return
	}

	pk, ok := pub.(*rsa.PublicKey)
	if !ok {
		err = fmt.Errorf("Value returned from ParsePKIXPublicKey was not an RSA public key")
	}

	return
}
