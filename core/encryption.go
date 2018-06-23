package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"

	sysid "github.com/foxcpp/go-sysid"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/blake2b"
)

/*
Master key is either Argon2-processed user password or
Argon2-processed output of go-sysid library (github.com/foxcpp/go-sysid).

Reason for go-sysid usage:
 Provide minimal level of security even without user's effort. However,
 go-sysid-based key can change if system information changes so we should
 not use it for anything we can't recover using other ways:
 - If it's account's password - we can ask it from user.
 - If it's messages cache - we can just redownload it.

This is how we store encrypted data:
+------------+------AES-256-CFB-encrypted-----+
|		     |+-------------+----------------+|
|	  IV 	 ||	BLAKE2b sum |      data		 ||
|			 |+-------------+----------------+|
+------------+--------------------------------+
*/

const (
	checksumSize = 512 / 8 // BLAKE2b-512
)

var (
	ErrInvalidKey  = errors.New("decrypt: invalid key or corrupted data")
	ErrInvalidSalt = errors.New("preparekey: invalid salt")
)

func (c *Client) SetMasterPass(pass string) error {
	havePass := (pass != "")
	c.GlobalCfg.Encryption.UseMasterPass = &havePass

	// Do not reuse salt.
	salt := make([]byte, 64)
	_, err := rand.Read(salt)
	if err != nil {
		return err
	}
	c.GlobalCfg.Encryption.MasterKeySalt = hex.EncodeToString(salt)

	// Re-encrypt all things.
	for acc, _ := range c.caches {
		c.caches[acc].dirty = true
		c.saveFullCache(acc)
	}
	for acc, conf := range c.serverCfgs {
		cfg := c.Accounts[acc]
		cfg.Credentials.Pass = hex.EncodeToString(c.EncryptUsingMaster([]byte(conf.imap.Pass)))
		c.Accounts[acc] = cfg

	}

	return c.prepareMasterKey(pass)
}

func (c *Client) prepareMasterKey(pass string) error {
	if pass == "" {
		Logger.Println("Alert: No master password set. Falling back to system information-dervied key")
		passB, err := sysid.SysID()
		if err != nil {
			return err
		}
		pass = string(passB)
	}

	if c.GlobalCfg.Encryption.MasterKeySalt == "" {
		salt := make([]byte, 64)
		_, err := rand.Read(salt)
		if err != nil {
			return err
		}
		c.GlobalCfg.Encryption.MasterKeySalt = hex.EncodeToString(salt)
	}

	salt, err := hex.DecodeString(c.GlobalCfg.Encryption.MasterKeySalt)
	if err != nil {
		return ErrInvalidSalt
	}

	c.masterKey = argon2.IDKey([]byte(pass), salt, 1, 64*1024, 2, 32)
	return nil
}

func (c *Client) EncryptUsingMaster(blob []byte) []byte {
	key := c.masterKey

	alg, err := aes.NewCipher(key)
	iv := make([]byte, alg.BlockSize())
	_, err = rand.Read(iv)
	if err != nil {
		Logger.Println("CRNG read fail:", err)
		panic(err) // TODO: Handle it in more graceful way?
	}
	enc := cipher.NewCFBEncrypter(alg, iv)

	// We need a checksum to confirm correctness of decrypted data later.
	sum := blake2b.Sum512(blob)

	plaintext := append(sum[:], blob...)

	ciphertext := make([]byte, len(plaintext))
	enc.XORKeyStream(ciphertext, plaintext)
	result := append(iv, ciphertext...)
	return result
}

func (c *Client) DecryptUsingMaster(blob []byte) ([]byte, error) {
	key := c.masterKey

	alg, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	iv := blob[:alg.BlockSize()]
	dec := cipher.NewCFBDecrypter(alg, iv)

	plaintext := make([]byte, len(blob[alg.BlockSize():]))
	dec.XORKeyStream(plaintext, blob[alg.BlockSize():])

	checksum := plaintext[:checksumSize]
	res := plaintext[checksumSize:]
	realSum := blake2b.Sum512(res)
	if subtle.ConstantTimeCompare(realSum[:], checksum) != 1 {
		return nil, ErrInvalidKey
	}
	return res, nil
}
