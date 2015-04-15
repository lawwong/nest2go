package uuid

import (
	"crypto/rand"
	"fmt"
)

const UuidSize = 16

var ErrInvalidLength = fmt.Errorf("wrong length")
var empty = Uuid{}

// create a new uuid v4
func New() *Uuid {
	u := Rand()
	return &u
}

func Rand() Uuid {
	u := Uuid{}
	_, err := rand.Read(u[:])
	if err != nil {
		panic(err)
	}

	u[8] = (u[8] | 0x80) & 0xBf
	u[6] = (u[6] | 0x40) & 0x4f
	return u
}

func FromByteSlice(src []byte) (u Uuid, err error) {
	if l := len(src); l != UuidSize {
		err = ErrInvalidLength
		return
	}
	for i := 0; i < UuidSize; i++ {
		u[i] = src[i]
	}
	return
}

type Uuid [UuidSize]byte

func (u Uuid) String() string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[:4], u[4:6], u[6:8], u[8:10], u[10:])
}

func (u Uuid) ByteSlice() []byte {
	return u[:]
}

func (u Uuid) IsEmpty() bool {
	return u == empty
}
