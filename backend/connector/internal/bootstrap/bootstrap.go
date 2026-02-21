package bootstrap

import _ "embed"

//go:embed ca.pem
var caPEM []byte

func CAPEM() []byte {
	return caPEM
}
