package common

type ConnType int

const (
	TLS ConnType = iota
	STARTTLS
)

type ServConfig struct {
	Host       string
	Port       uint16
	ConnType   ConnType
	User, Pass string
}
