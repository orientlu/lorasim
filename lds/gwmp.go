package lds

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"

	log "github.com/sirupsen/logrus"
)

type GwmpRxpk struct {
	PhyPayload []byte  `json:"data"`
	Channel    int     `json:"chan"`
	CodeRate   string  `json:"codr"`
	CrcStatus  int     `json:"stat"`
	DataRate   string  `json:"datr"`
	Modu       string  `json:"modu"`
	Frequency  float32 `json:"freq"`
	LoRaSNR    float32 `json:"lsnr"`
	RfChain    int     `json:"rfch"`
	Rssi       int     `json:"rssi"`
	Size       int     `json:"size"`
	Timestamp  int32   `json:"tmst"`
}

type GwmpRxpkWrapper struct {
	Rxpk []GwmpRxpk `json:"rxpk"`
}

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

//Uplink sends an uplink message as if it was sent from a lora-gateway-bridge. Works only for ABP devices with relaxed frame counter.
func (d *Device) UplinkMessageGWMP(mType lorawan.MType, fPort uint8, rxInfo *GwmpRxpk, txInfo *gw.UplinkTXInfo, payload []byte, gwMAC string, bandName band.Name, dr DataRate) ([]byte, error) {

	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: mType,
			Major: d.Major,
		},
		MACPayload: &lorawan.MACPayload{
			FHDR: lorawan.FHDR{
				DevAddr: d.DevAddr,
				FCtrl: lorawan.FCtrl{
					ADR:       false,
					ADRACKReq: false,
					ACK:       false,
				},
				FCnt:  d.UlFcnt,
				FOpts: []lorawan.Payload{}, // you can leave this out when there is no MAC command to send
			},
			FPort:      &fPort,
			FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: payload}},
		},
	}

	if err := phy.EncryptFRMPayload(d.AppSKey); err != nil {
		fmt.Printf("encrypt frm payload: %s", err)
		return nil, err
	}

	if d.MACVersion == lorawan.LoRaWAN1_0 {
		if err := phy.SetUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, d.NwkSEncKey, d.NwkSEncKey); err != nil {
			fmt.Printf("set uplink mic error: %s", err)
			return nil, err
		}
		phy.ValidateUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, d.NwkSEncKey, d.NwkSEncKey)
	} else if d.MACVersion == lorawan.LoRaWAN1_1 {
		//Get the band.
		b, err := band.GetConfig(bandName, false, lorawan.DwellTime400ms)
		if err != nil {
			return nil, err
		}
		//Get DR index from a dr.
		dataRate := band.DataRate{
			Modulation:   band.Modulation(dr.Modulation),
			SpreadFactor: dr.SpreadFactor,
			Bandwidth:    dr.Bandwidth,
			BitRate:      dr.BitRate,
		}
		txDR, err := b.GetDataRateIndex(true, dataRate)
		if err != nil {
			return nil, err
		}
		//Get tx ch.
		var txCh int
		for _, defaultChannel := range []bool{true, false} {
			i, err := b.GetUplinkChannelIndex(int(txInfo.Frequency), defaultChannel)
			if err != nil {
				continue
			}

			c, err := b.GetUplinkChannel(i)
			if err != nil {
				return nil, err
			}

			// there could be multiple channels using the same frequency, but with different data-rates.
			// eg EU868:
			//  channel 1 (868.3 DR 0-5)
			//  channel x (868.3 DR 6)
			if c.MinDR <= txDR && c.MaxDR >= txDR {
				txCh = i
			}
		}
		//Encrypt fOPts.
		if err := phy.EncryptFOpts(d.NwkSEncKey); err != nil {
			log.Errorf("encrypt fopts error: %s", err)
			return nil, err
		}

		//Now set the MIC.
		if err := phy.SetUplinkDataMIC(lorawan.LoRaWAN1_1, 0, uint8(txDR), uint8(txCh), d.FNwkSIntKey, d.SNwkSIntKey); err != nil {
			log.Errorf("set uplink mic error: %s", err)
			return nil, err
		}

		log.Printf("Got MIC: %s\n", phy.MIC)

	} else {
		return nil, errors.New("unknown lorawan version")
	}

	phyBytes, err := phy.MarshalBinary()
	if err != nil {
		if err != nil {
			fmt.Printf("marshal binary error: %s", err)
			return nil, err
		}
	}

	log.Printf("phycal payload hex: % x\n", phyBytes)

	rxInfo.PhyPayload = phyBytes

	rx := GwmpRxpkWrapper{}
	rx.Rxpk = append(rx.Rxpk, *rxInfo)

	msg, err := json.Marshal(rx)
	if err != nil {
		log.Printf("couldn't marshal msg: %s\n", err)
		return nil, err
	}

	return msg, nil

}
