package vnc

import (
	"crypto/aes"
	"crypto/des" // #nosec G502 -- VNC password authentication is defined with DES.
	"crypto/md5" // #nosec G501 -- Apple Remote Desktop authentication derives its key with MD5.
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"math/bits"
)

const (
	vncChallengeLength = 16
	ardCredentialBytes = 128
	ardFieldBytes      = 64
	// maxARDKeyLength bounds the Diffie-Hellman prime a server may send. macOS
	// uses 128 bytes.
	maxARDKeyLength = 1024
)

// ChallengeResponse encrypts a VNC authentication challenge with the password,
// as RFB defines it: the first eight bytes of the password, each with its bits
// reversed, form a DES key. A server checks a client's answer the same way.
func ChallengeResponse(password string, challenge []byte) ([]byte, error) {
	key := make([]byte, 8)
	copy(key, password)
	for i, b := range key {
		key[i] = bits.Reverse8(b)
	}
	block, err := des.NewCipher(key) // #nosec G405 -- required by the protocol.
	if err != nil {
		return nil, err
	}
	response := make([]byte, len(challenge))
	for i := 0; i+block.BlockSize() <= len(challenge); i += block.BlockSize() {
		block.Encrypt(response[i:], challenge[i:])
	}
	return response, nil
}

func authenticateVNC(conn io.ReadWriter, password string) error {
	challenge := make([]byte, vncChallengeLength)
	if _, err := io.ReadFull(conn, challenge); err != nil {
		return fmt.Errorf("reading the VNC challenge: %w", err)
	}
	response, err := ChallengeResponse(password, challenge)
	if err != nil {
		return err
	}
	if _, err := conn.Write(response); err != nil {
		return fmt.Errorf("sending the VNC response: %w", err)
	}
	return nil
}

// ardParams are what an Apple Remote Desktop server sends to start
// authentication: a Diffie-Hellman group and its public key.
type ardParams struct {
	generator *big.Int
	prime     *big.Int
	serverKey *big.Int
	keyLength int
}

func readARDParams(r io.Reader) (ardParams, error) {
	var header struct {
		Generator uint16
		KeyLength uint16
	}
	if err := binary.Read(r, binary.BigEndian, &header); err != nil {
		return ardParams{}, fmt.Errorf("reading the Apple Remote Desktop parameters: %w", err)
	}
	if header.KeyLength == 0 || header.KeyLength > maxARDKeyLength {
		return ardParams{}, fmt.Errorf("unexpected Apple Remote Desktop key length %d", header.KeyLength)
	}
	buffer := make([]byte, 2*int(header.KeyLength))
	if _, err := io.ReadFull(r, buffer); err != nil {
		return ardParams{}, fmt.Errorf("reading the Apple Remote Desktop key: %w", err)
	}
	params := ardParams{
		generator: new(big.Int).SetUint64(uint64(header.Generator)),
		prime:     new(big.Int).SetBytes(buffer[:header.KeyLength]),
		serverKey: new(big.Int).SetBytes(buffer[header.KeyLength:]),
		keyLength: int(header.KeyLength),
	}
	two := big.NewInt(2)
	upper := new(big.Int).Sub(params.prime, two)
	// A key outside [2, p-2] would make the shared secret predictable.
	if params.prime.Cmp(big.NewInt(5)) < 0 || params.serverKey.Cmp(two) < 0 || params.serverKey.Cmp(upper) > 0 {
		return ardParams{}, errors.New("the server sent an invalid Apple Remote Desktop key")
	}
	return params, nil
}

// ardCredentials lays out the user name and password as the server expects
// them: two NUL-terminated 64-byte fields, padded with random bytes.
func ardCredentials(login Credentials) ([]byte, error) {
	block := make([]byte, ardCredentialBytes)
	if _, err := rand.Read(block); err != nil {
		return nil, fmt.Errorf("generating padding: %w", err)
	}
	put := func(offset int, value string) {
		field := []byte(value)
		if len(field) > ardFieldBytes-1 {
			field = field[:ardFieldBytes-1]
		}
		copy(block[offset:], field)
		block[offset+len(field)] = 0
	}
	put(0, login.Username)
	put(ardFieldBytes, login.Password)
	return block, nil
}

// ardResponse computes the client's answer: the credentials encrypted with
// AES-128 under the MD5 of the shared secret, followed by the client's public
// key. private is the client's Diffie-Hellman private key.
func ardResponse(params ardParams, private *big.Int, login Credentials) ([]byte, error) {
	public := new(big.Int).Exp(params.generator, private, params.prime)
	shared := new(big.Int).Exp(params.serverKey, private, params.prime)
	sharedBytes := shared.FillBytes(make([]byte, params.keyLength))
	key := md5.Sum(sharedBytes) // #nosec G401 -- required by the protocol.

	plain, err := ardCredentials(login)
	if err != nil {
		return nil, err
	}
	cipher, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	response := make([]byte, ardCredentialBytes+params.keyLength)
	// The protocol uses ECB: each block is encrypted on its own.
	for i := 0; i < ardCredentialBytes; i += aes.BlockSize {
		cipher.Encrypt(response[i:], plain[i:])
	}
	public.FillBytes(response[ardCredentialBytes:])
	return response, nil
}

func authenticateARD(conn io.ReadWriter, login Credentials) error {
	params, err := readARDParams(conn)
	if err != nil {
		return err
	}
	// The private key is drawn from [2, p-2].
	limit := new(big.Int).Sub(params.prime, big.NewInt(3))
	private, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return fmt.Errorf("generating the Apple Remote Desktop key: %w", err)
	}
	private.Add(private, big.NewInt(2))
	response, err := ardResponse(params, private, login)
	if err != nil {
		return err
	}
	if _, err := conn.Write(response); err != nil {
		return fmt.Errorf("sending the Apple Remote Desktop response: %w", err)
	}
	return nil
}
