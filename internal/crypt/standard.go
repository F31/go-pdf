// Package crypt implements the PDF Standard security handler.
package crypt

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rc4"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"

	"github.com/F31/go-pdf/internal/errs"
)

var padding = []byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41,
	0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80,
	0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

// Filter describes one crypt filter dictionary.
type Filter struct {
	CFM    string
	Length int
}

// Options is the parsed Standard encryption dictionary.
type Options struct {
	V, R, Length, P int
	O, U, OE, UE    []byte
	Perms           []byte
	ID              []byte
	EncryptMetadata bool
	StmF, StrF      string
	Filters         map[string]Filter
}

// Handler decrypts strings and streams for one document.
type Handler struct {
	v, r            int
	key             []byte
	stmF, strF      string
	filters         map[string]Filter
	encryptMetadata bool
}

// New validates a user or owner password and derives the file key.
func New(opts Options, password string) (*Handler, error) {
	if opts.V >= 5 || opts.R >= 5 {
		return newAES256(opts, password)
	}
	if opts.V > 4 || opts.R > 4 || opts.V < 1 || opts.R < 2 {
		return nil, errs.Wrap(errs.ErrUnsupported, "CRY-UNSUPPORTED", "unsupported standard encryption", -1, 0, -1)
	}
	n := opts.Length / 8
	if opts.R == 2 {
		n = 5
	}
	if n < 5 {
		n = 5
	}
	if n > 16 {
		n = 16
	}
	if len(opts.O) < 32 || len(opts.U) < 16 {
		return nil, errs.Wrap(errs.ErrCorrupt, "CRY-DICT", "invalid standard encryption dictionary", -1, 0, -1)
	}
	key, ok := validateUser(opts, padPassword([]byte(password)), n)
	if !ok {
		if password == "" {
			return nil, errs.Wrap(errs.ErrPassword, "CRY-PASSWORD", "password required or incorrect", -1, 0, -1)
		}
		ownerKey := ownerKey(opts, padPassword([]byte(password)), n)
		userPad := decryptRC4(ownerKey, opts.O)
		for i := 1; i < 20; i++ {
			userPad = decryptRC4(xorKey(ownerKey, byte(i)), userPad)
		}
		key, ok = validateUser(opts, userPad, n)
		if !ok {
			return nil, errs.Wrap(errs.ErrPassword, "CRY-PASSWORD", "password required or incorrect", -1, 0, -1)
		}
	}
	return &Handler{
		v: opts.V, r: opts.R, key: key, stmF: opts.StmF, strF: opts.StrF,
		filters: opts.Filters, encryptMetadata: opts.EncryptMetadata,
	}, nil
}

// MetadataEncrypted reports whether metadata streams are encrypted.
func (h *Handler) MetadataEncrypted() bool { return h == nil || h.encryptMetadata }

func newAES256(opts Options, password string) (*Handler, error) {
	if len(opts.U) < 48 || len(opts.O) < 48 || len(opts.UE) < 32 || len(opts.OE) < 32 {
		return nil, errs.Wrap(errs.ErrCorrupt, "CRY-DICT", "invalid AES-256 encryption dictionary", -1, 0, -1)
	}
	pwd := []byte(saslprep(password))
	if len(pwd) > 127 {
		pwd = pwd[:127]
	}
	key := verifyAES256User(opts, pwd)
	if key == nil {
		key = verifyAES256Owner(opts, pwd)
	}
	if key == nil {
		return nil, errs.Wrap(errs.ErrPassword, "CRY-PASSWORD", "password required or incorrect", -1, 0, -1)
	}
	h := &Handler{v: opts.V, r: opts.R, key: key, stmF: opts.StmF, strF: opts.StrF, filters: opts.Filters, encryptMetadata: opts.EncryptMetadata}
	if len(opts.Perms) >= 16 && !verifyPerms(key, opts.Perms, opts.P, opts.EncryptMetadata) {
		return nil, errs.Wrap(errs.ErrCorrupt, "CRY-PERMS", "invalid encryption permissions", -1, 0, -1)
	}
	return h, nil
}

func verifyAES256User(opts Options, password []byte) []byte {
	if !bytes.Equal(aes256Hash(opts.R, password, opts.U[32:40], nil), opts.U[:32]) {
		return nil
	}
	ik := aes256Hash(opts.R, password, opts.U[40:48], nil)
	return aesCBCDecryptZero(ik, opts.UE[:32])
}

func verifyAES256Owner(opts Options, password []byte) []byte {
	if !bytes.Equal(aes256Hash(opts.R, password, opts.O[32:40], opts.U[:48]), opts.O[:32]) {
		return nil
	}
	ik := aes256Hash(opts.R, password, opts.O[40:48], opts.U[:48])
	return aesCBCDecryptZero(ik, opts.OE[:32])
}

func aes256Hash(r int, password, salt, u []byte) []byte {
	h := sha256.Sum256(append(append(append([]byte{}, password...), salt...), u...))
	k := h[:]
	if r < 6 {
		return append([]byte(nil), k...)
	}
	for count := 1; ; count++ {
		block := append(append(append([]byte{}, password...), k...), u...)
		rep := make([]byte, 0, len(block)*64)
		for i := 0; i < 64; i++ {
			rep = append(rep, block...)
		}
		e := aesCBCEncryptNoPad(k[:16], k[16:32], rep)
		if e == nil {
			return nil
		}
		sum := 0
		for _, b := range e[:16] {
			sum += int(b)
		}
		switch sum % 3 {
		case 0:
			x := sha256.Sum256(e)
			k = x[:]
		case 1:
			x := sha512.Sum384(e)
			k = x[:]
		default:
			x := sha512.Sum512(e)
			k = x[:]
		}
		if count >= 64 && int(e[len(e)-1]) <= count-32 {
			break
		}
	}
	return append([]byte(nil), k[:32]...)
}

func verifyPerms(key, perms []byte, p int, meta bool) bool {
	if len(perms) < 16 || len(key) != 32 {
		return false
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return false
	}
	out := make([]byte, 16)
	for i := 0; i < 16; i += 16 {
		block.Decrypt(out[i:i+16], perms[i:i+16])
	}
	if string(out[9:12]) != "adb" {
		return false
	}
	if binary.LittleEndian.Uint32(out[:4]) != uint32(p) {
		return false
	}
	if (meta && out[8] != 'T') || (!meta && out[8] != 'F') {
		return false
	}
	return true
}

func aesCBCDecryptZero(key, data []byte) []byte {
	if len(key) != 32 || len(data) < 16 || len(data)%16 != 0 {
		return nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil
	}
	out := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, make([]byte, 16)).CryptBlocks(out, data)
	return out
}

func aesCBCEncryptNoPad(key, iv, data []byte) []byte {
	if len(iv) != 16 || len(data) == 0 || len(data)%16 != 0 {
		return nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil
	}
	out := make([]byte, len(data))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, data)
	return out
}

// DecryptString decrypts a PDF string object.
func (h *Handler) DecryptString(num, gen int, data []byte) ([]byte, error) {
	if h == nil {
		return data, nil
	}
	return h.decrypt(num, gen, data, h.strF)
}

// DecryptStream decrypts a stream body.
func (h *Handler) DecryptStream(num, gen int, data []byte) ([]byte, error) {
	if h == nil {
		return data, nil
	}
	return h.decrypt(num, gen, data, h.stmF)
}

func (h *Handler) decrypt(num, gen int, data []byte, filter string) ([]byte, error) {
	method := h.method(filter)
	switch method {
	case "", "Identity", "None":
		return data, nil
	case "V2":
		key := h.objectKey(num, gen, false)
		return decryptRC4(key, data), nil
	case "AESV2":
		key := h.objectKey(num, gen, true)
		return decryptAES(key, data)
	case "AESV3":
		return decryptAES(h.key, data)
	default:
		return nil, errs.Wrap(errs.ErrUnsupported, "CRY-FILTER", "unsupported crypt filter", -1, 0, -1)
	}
}

func (h *Handler) method(name string) string {
	if name == "" || name == "Identity" {
		if h.v < 4 {
			return "V2"
		}
		if h.v >= 5 {
			return "AESV3"
		}
		return "Identity"
	}
	f, ok := h.filters[name]
	if !ok {
		if h.v < 4 {
			return "V2"
		}
		return "Identity"
	}
	return f.CFM
}

func (h *Handler) objectKey(num, gen int, aes bool) []byte {
	hsh := md5.New()
	hsh.Write(h.key)
	var b [5]byte
	b[0] = byte(num)
	b[1] = byte(num >> 8)
	b[2] = byte(num >> 16)
	b[3] = byte(gen)
	b[4] = byte(gen >> 8)
	hsh.Write(b[:])
	if aes {
		hsh.Write([]byte("sAlT"))
	}
	sum := hsh.Sum(nil)
	n := len(h.key) + 5
	if n > 16 {
		n = 16
	}
	return sum[:n]
}

func validateUser(opts Options, password []byte, n int) ([]byte, bool) {
	key := fileKey(opts, password, n)
	if opts.R == 2 {
		u := decryptRC4(key, padding)
		return key, bytes.Equal(u, opts.U[:32])
	}
	h := md5.New()
	h.Write(padding)
	h.Write(opts.ID)
	u := decryptRC4(key, h.Sum(nil))
	for i := 1; i < 20; i++ {
		u = decryptRC4(xorKey(key, byte(i)), u)
	}
	return key, bytes.Equal(u[:16], opts.U[:16])
}

func ownerKey(opts Options, password []byte, n int) []byte {
	sum := md5.Sum(password)
	key := sum[:]
	if opts.R >= 3 {
		for i := 0; i < 50; i++ {
			sum = md5.Sum(key[:n])
			key = sum[:]
		}
	}
	return key[:n]
}

func fileKey(opts Options, password []byte, n int) []byte {
	h := md5.New()
	h.Write(padPassword(password))
	h.Write(opts.O)
	var p [4]byte
	binary.LittleEndian.PutUint32(p[:], uint32(opts.P))
	h.Write(p[:])
	h.Write(opts.ID)
	if opts.R >= 4 && !opts.EncryptMetadata {
		h.Write([]byte{0xff, 0xff, 0xff, 0xff})
	}
	key := h.Sum(nil)
	if opts.R >= 3 {
		for i := 0; i < 50; i++ {
			sum := md5.Sum(key[:n])
			key = sum[:]
		}
	}
	return key[:n]
}

func padPassword(p []byte) []byte {
	out := make([]byte, 32)
	if len(p) >= 32 {
		copy(out, p[:32])
		return out
	}
	copy(out, p)
	copy(out[len(p):], padding)
	return out
}

func decryptRC4(key, data []byte) []byte {
	c, err := rc4.NewCipher(key)
	if err != nil {
		return nil
	}
	out := make([]byte, len(data))
	c.XORKeyStream(out, data)
	return out
}

func xorKey(key []byte, v byte) []byte {
	out := make([]byte, len(key))
	for i := range key {
		out[i] = key[i] ^ v
	}
	return out
}

func decryptAES(key, data []byte) ([]byte, error) {
	if len(data) < aes.BlockSize {
		return nil, errs.Wrap(errs.ErrCorrupt, "CRY-AES", "AES stream is truncated", -1, 0, -1)
	}
	if (len(data)-aes.BlockSize)%aes.BlockSize != 0 {
		return nil, errs.Wrap(errs.ErrCorrupt, "CRY-AES", "AES stream is not block aligned", -1, 0, -1)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(data)-aes.BlockSize)
	cipher.NewCBCDecrypter(block, data[:aes.BlockSize]).CryptBlocks(out, data[aes.BlockSize:])
	if len(out) == 0 {
		return out, nil
	}
	p := int(out[len(out)-1])
	if p <= 0 || p > aes.BlockSize || p > len(out) {
		return nil, errs.Wrap(errs.ErrCorrupt, "CRY-AES", "invalid AES padding", -1, 0, -1)
	}
	for _, b := range out[len(out)-p:] {
		if int(b) != p {
			return nil, errs.Wrap(errs.ErrCorrupt, "CRY-AES", "invalid AES padding", -1, 0, -1)
		}
	}
	return out[:len(out)-p], nil
}
