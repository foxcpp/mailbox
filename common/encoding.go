package common

type Encoding interface {
	Encode(dst, src []byte)
	Decode(dst, src []byte) (int, error)
	EncodedLen(n int) int
	DecodedLen(n int) int
}

type DummyEncoding struct{}

func (d DummyEncoding) Encode(dst, src []byte) {
	copy(dst, src)
}

func (d DummyEncoding) Decode(dst, src []byte) (int, error) {
	return copy(dst, src), nil
}

func (d DummyEncoding) EncodedLen(n int) int {
	return n
}

func (d DummyEncoding) DecodedLen(n int) int {
	return n
}
