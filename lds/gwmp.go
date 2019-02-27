package lds

import (
	"encoding/hex"
	log "github.com/sirupsen/logrus"
)


func GenGWMP(gweui string) [12]byte {
	var ret [12]byte
	if len(gweui) != 16 {
		log.Printf("error gweui %s\n", gweui)
		gweui = "60c5a8fffe6f7473"
	}
	head, _ := hex.DecodeString("024da800")
	gweuibytes, _ := hex.DecodeString(gweui)

	copy(ret[:], head)
	copy(ret[4:], gweuibytes)

	return ret
	
}
