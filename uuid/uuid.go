package uuid

import (
	"crypto/rand"
	"fmt"
)

const UuidSize = 16

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
		err = Error(fmt.Sprint("[uuid.ByteSlice] got len(src)=", l, " not ", UuidSize))
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

type Error string

func (e Error) Error() string {
	return string(e)
}
