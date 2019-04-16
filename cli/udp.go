package main

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	b64 "encoding/base64"
	"encoding/hex"
	"encoding/json"

	"math/rand"

	"github.com/madjack101/lds/lds"

	"time"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/lorawan"

	log "github.com/sirupsen/logrus"
)

var (
	conf *tomlConfig
	dev  *lds.Device
)

type udp struct {
	Server string `toml:"server"`
	Conn   *net.UDPConn
	Mutex  *sync.Mutex
}

func (u *udp) Send(msg []byte) {
	u.Mutex.Lock()
	_, err := u.Conn.Write(msg)

	if err != nil {
		log.Println("udp send failed:", err)
	}

	u.Mutex.Unlock()
}

func (u *udp) init(server string) {
	u.Mutex = &sync.Mutex{}

	addr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		log.Printf("Can't resolve address: ", err)
		os.Exit(1)
	}

	u.Conn, err = net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Println("Can't dial: ", err)
		os.Exit(1)
	}

	go func(u *udp) {
		appSKey, err := lds.HexToKey(config.Device.AppSKey)
		nwkSkey, err := lds.HexToKey(config.Device.NwkSEncKey)
		if err != nil {
			log.Printf("appskey error: %s", err)
		}

		gweuibytes, _ := hex.DecodeString(config.GW.MAC)
		for {
			data := make([]byte, 1024)
			rlen, err := u.Conn.Read(data)

			if err != nil {
				log.Println("failed to read UDP msg because of ", err)
				continue
			}

			if rlen > 0 {
				plen := rlen
				if plen > 4 {
					plen = 4
				}

				log.Printf("downlink message head: % x", data[:plen])
			}

			if rlen > 12 && data[3] == '\x03' {
				var js map[string]interface{}

				if err := json.Unmarshal(data[4:rlen], &js); err != nil {
					log.Printf("json parse error: %s\n", err.Error())
					continue
				}

				payloadb64 := js["txpk"].(map[string]interface{})["data"].(string)

				phy := lorawan.PHYPayload{}
				_ = phy.UnmarshalText([]byte(payloadb64))

				if dev.MACVersion == lorawan.LoRaWAN1_1 {
					phy.DecryptFOpts(nwkSkey)
				}

				//parse maccommand
				macPL, ok := phy.MACPayload.(*lorawan.MACPayload)
				if !ok {
					log.Printf("lorawan: MACPayload must be of type *MACPayload\n")
					continue
				}

				if macPL.FPort != nil && *macPL.FPort == 0 {
					phy.DecryptFRMPayload(nwkSkey)
				} else {
					phy.DecryptFRMPayload(appSKey)
				}

				// print phy json

				phystr, _ := json.Marshal(phy)
				log.Printf("downlink phypayload: %s\n", phystr)

				b, _ := b64.StdEncoding.DecodeString(payloadb64)
				js["txpk"].(map[string]interface{})["data"] = fmt.Sprintf("% x", b)

				newpayload, _ := json.Marshal(js)
				log.Printf("downlink macpayload: %s\n", newpayload)

				//answer reply
				reply := append(data[0:4], gweuibytes...)
				reply[3] = '\x05'
				u.Send(reply)
				log.Printf("downlink reply: % x\n", reply)

				// replay linkadr request
				if macPL.FPort != nil && *macPL.FPort == 0 {
					if strings.Contains(string(phystr), "LinkADRReq") {
						// u.Send
						rephy := []byte{'\x03', '\x07', '\x03', '\x07', '\x03', '\x07',
							'\x03', '\x07', '\x03', '\x07', '\x03', '\x07', '\x06', '\xff', '\x06'}
						// msg, _ := genPacketBytes(conf, dev, []byte("\x00"), 8, 15, rephy)
						msg, _ := genPacketBytes(conf, dev, rephy, 0, 0, rephy)

						msg[3] = '\x00'
						log.Printf("LinkAdrAns: %s", msg[12:])
						u.Send(msg)
					} else if strings.Contains(string(phystr), "DevStatusReq") {
						// u.Send
						rephy := []byte{'\x06', '\xff', '\x0e'}
						// msg, _ := genPacketBytes(conf, dev, []byte("\x00"), 8, 15, rephy)
						msg, _ := genPacketBytes(conf, dev, rephy, 0, 0, nil)

						msg[3] = '\x00'
						log.Printf("DevStatusAns: %s", msg[12:])
						u.Send(msg)
					}

				}

				var foptreply []byte
				if strings.Contains(string(phystr), "RXParamSetupReq") {
					// u.Send
					macAns := lorawan.MACCommand{
						CID:     lorawan.RXParamSetupAns,
						Payload: &lorawan.RXParamSetupAnsPayload{true, true, true},
					}
					rephy, _ := macAns.MarshalBinary()
					foptreply = append(foptreply, rephy...)
				}
				if strings.Contains(string(phystr), "RXTimingSetupReq") {
					foptreply = append(foptreply, byte(lorawan.RXTimingSetupAns))
				}

				if len(foptreply) > 0 {
					log.Printf("fopt uplink: % x\n", foptreply)
					msg, _ := genPacketBytes(conf, dev, []byte("\x00"), 8, 15, foptreply)
					msg[3] = '\x00'
					log.Printf("MacCommand Ans: %s\n", msg[12:])
					u.Send(msg)
				}

			}

		}
	}(u)

	// send gwstat per 30s
	go func(u *udp) {
		for {
			msg := lds.GwStatPacket()
			log.Printf("send gwstat: % x\n", msg)

			u.Send(msg)

			time.Sleep(time.Duration(30000) * time.Millisecond)
		}

	}(u)

	// send gw keepalived per 3s
	go func(u *udp) {
		for {
			msg := lds.GenGWKeepalived()
			log.Printf("send gwkeep: % x\n", msg)

			u.Send(msg)

			time.Sleep(time.Duration(3000) * time.Millisecond)
		}

	}(u)

}

func genPacketBytes(config *tomlConfig, d *lds.Device, payload []byte, fport uint8, fOptlen uint8, fOpts []byte) ([]byte, error) {
	dataRate := &lds.DataRate{
		Bandwidth:    config.DR.Bandwith,
		Modulation:   "LORA",
		SpreadFactor: config.DR.SpreadFactor,
		BitRate:      config.DR.BitRate,
	}

	dataRateStr := fmt.Sprintf("SF%dBW%d", dataRate.SpreadFactor, dataRate.Bandwidth)

	rxInfo := &lds.GwmpRxpk{
		Channel:   config.RXInfo.Channel,
		CodeRate:  config.RXInfo.CodeRate,
		CrcStatus: config.RXInfo.CrcStatus,
		DataRate:  dataRateStr,
		Modu:      dataRate.Modulation,
		Frequency: float32(config.RXInfo.Frequency) / 1000000.0,
		LoRaSNR:   float32(config.RXInfo.LoRaSNR),
		RfChain:   config.RXInfo.RfChain,
		Rssi:      config.RXInfo.Rssi,
		Size:      len(payload),
		Tmst:      uint32(time.Now().UnixNano() / 1000),
	}

	// now := time.Now()
	// rxTime := ptypes.TimestampNow()
	// tsge := 	ptypes.DurationProto(now.Sub(time.Time{}))

	lmi := &gw.LoRaModulationInfo{
		Bandwidth:       uint32(dataRate.Bandwidth),
		SpreadingFactor: uint32(dataRate.SpreadFactor),
		CodeRate:        rxInfo.CodeRate,
	}

	umi := &gw.UplinkTXInfo_LoraModulationInfo{
		LoraModulationInfo: lmi,
	}

	utx := gw.UplinkTXInfo{
		Frequency:      uint32(config.RXInfo.Frequency),
		ModulationInfo: umi,
	}

	//////
	mType := lorawan.UnconfirmedDataUp
	if config.Device.MType > 0 {
		mType = lorawan.ConfirmedDataUp
	}

	//Now send an uplink
	msg, err := d.UplinkMessageGWMP(mType, fport, rxInfo, &utx, payload, config.GW.MAC, config.Band.Name, *dataRate, fOptlen, fOpts)

	gwmphead := lds.GenGWMP(config.GW.MAC)
	msg = append(gwmphead[:], msg[:]...)

	return msg, err

}

func RunUdp(config *tomlConfig) {
	lds.InitGWMP()

	if config.UDP.Server != "" {
		config.UDP.init(config.UDP.Server)
	} else {
		os.Exit(-1)
	}

	log.Println("Connection established.")

	//Build your node with known keys (ABP).
	nwkSEncHexKey := config.Device.NwkSEncKey
	sNwkSIntHexKey := config.Device.SNwkSIntKey
	fNwkSIntHexKey := config.Device.FNwkSIntKey
	appSHexKey := config.Device.AppSKey
	devHexAddr := config.Device.Address
	devAddr, err := lds.HexToDevAddress(devHexAddr)
	if err != nil {
		log.Printf("dev addr error: %s", err)
	}

	nwkSEncKey, err := lds.HexToKey(nwkSEncHexKey)
	if err != nil {
		log.Printf("nwkSEncKey error: %s", err)
	}

	sNwkSIntKey, err := lds.HexToKey(sNwkSIntHexKey)
	if err != nil {
		log.Printf("sNwkSIntKey error: %s", err)
	}

	fNwkSIntKey, err := lds.HexToKey(fNwkSIntHexKey)
	if err != nil {
		log.Printf("fNwkSIntKey error: %s", err)
	}

	appSKey, err := lds.HexToKey(appSHexKey)
	if err != nil {
		log.Printf("appskey error: %s", err)
	}

	devEUI, err := lds.HexToEUI(config.Device.EUI)
	if err != nil {
		return
	}

	nwkHexKey := config.Device.NwkKey
	appHexKey := config.Device.AppKey

	appKey, err := lds.HexToKey(appHexKey)
	if err != nil {
		return
	}
	nwkKey, err := lds.HexToKey(nwkHexKey)
	if err != nil {
		return
	}
	appEUI := [8]byte{0, 0, 0, 0, 0, 0, 0, 0}

	device := &lds.Device{
		DevEUI:      devEUI,
		DevAddr:     devAddr,
		NwkSEncKey:  nwkSEncKey,
		SNwkSIntKey: sNwkSIntKey,
		FNwkSIntKey: fNwkSIntKey,
		AppSKey:     appSKey,
		AppKey:      appKey,
		NwkKey:      nwkKey,
		AppEUI:      appEUI,
		UlFcnt:      0,
		DlFcnt:      0,
		Major:       lorawan.Major(config.Device.Major),
		MACVersion:  lorawan.MACVersion(config.Device.MACVersion),
	}

	device.SetMarshaler(config.Device.Marshaler)

	conf = config
	dev = device

	mult := 1

	for {
		if stop {
			stop = false
			return
		}
		payload := []byte{}

		if config.RawPayload.UseRaw {
			var pErr error
			payload, pErr = hex.DecodeString(config.RawPayload.Payload)
			if err != nil {
				log.Errorf("couldn't decode hex payload: %s\n", pErr)
				return
			}
		} else {
			for _, v := range config.DefaultData.Data {
				rand.Seed(time.Now().UnixNano() / 10000)
				if rand.Intn(10) < 5 {
					mult *= -1
				}
				num := float32(v[0])
				if config.DefaultData.Random {
					num = float32(v[0] + float64(mult)*rand.Float64()/100)
				}
				arr := lds.GenerateFloat(num, float32(v[1]), int32(v[2]))
				payload = append(payload, arr...)

			}
		}

		log.Printf("Bytes: % x\n", payload)

		// now := time.Now()
		// rxTime := ptypes.TimestampNow()
		// tsge := 	ptypes.DurationProto(now.Sub(time.Time{}))

		msg, err := genPacketBytes(config, device, payload, 1, 0, nil)
		if err != nil {
			log.Printf("couldn't generate uplink: %s\n", err)
			return
		}

		log.Printf("Upload message: %v\n", string(msg[12:]))

		// send by udp
		// log.Printf("msg: % x\n", msg)

		config.UDP.Send(msg)

		device.UlFcnt++

		time.Sleep(time.Duration(config.DefaultData.Interval) * time.Millisecond)

	}

}
