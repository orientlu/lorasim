package udpgw

import (
	b64 "encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"git.code.oa.com/orientlu/lorasim/cli/config"
	"git.code.oa.com/orientlu/lorasim/lds"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/lorawan"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
)

var conf = &config.C
var stop = false

var devs map[lorawan.DevAddr]*lds.Device

type udp struct {
	Conn          *net.UDPConn
	sendChan      chan *[]byte
	SendTime      map[lorawan.EUI64]map[uint32]time.Time
	SendTimeMutex *sync.Mutex
	wg            sync.WaitGroup
}

// UDP gw udp
var UDP udp

// FunMqttHandle ...
var FunMqttHandle MQTT.MessageHandler = func(client MQTT.Client, msg MQTT.Message) {
	var js map[string]interface{}
	if err := json.Unmarshal(msg.Payload(), &js); err == nil {
		ackFcnt := (uint32)(js["fCnt"].(float64))
		strEui := js["devEUI"].(string)
		devEui, err := lds.HexToEUI(strEui)
		if err != nil {
			log.Error("app mqtt HexToEUI error: ", err)
			return
		}

		UDP.SendTimeMutex.Lock()
		defer UDP.SendTimeMutex.Unlock()

		if sendTime, ok := UDP.SendTime[devEui][ackFcnt]; ok {
			log.Warningf("msg_elapsed[%s]: %d ms\n", strEui, time.Now().Sub(sendTime)/time.Millisecond)
			//log.Warningf("msg fcnt[%d] elapsed: %s\n", ackFcnt, time.Since(sendTime))
			delete(UDP.SendTime[devEui], ackFcnt)

		} else {
			log.Warningf("app mqtt msg fnct[%d] can not found", ackFcnt)
		}
	} else {
		log.Errorf("app mqtt msg json unmarshal error %s", err)
	}
}

func (u *udp) Send(msg []byte) {
	UDP.sendChan <- &msg
	log.Debug("udp package push to Channel: ", msg)
}

func sendUDPLoop() {
	for msg := range UDP.sendChan {
		_, err := UDP.Conn.Write(*msg)
		if err != nil {
			log.Error("udp send failed: ", err)
			continue
		}
		log.Debug("udp send package: ", msg)
	}
}

// receive udp package and send ack to ns
func readUDPLoop() {
	appSKey, err := lds.HexToKey(conf.DeviceComm.AppSKey)
	if err != nil {
		log.Errorf("appskey error: %s", err)
	}
	nwkSkey, err := lds.HexToKey(conf.DeviceComm.NwkSEncKey)
	if err != nil {
		log.Errorf("nwkskey error: %s", err)
	}

	gweuibytes, _ := hex.DecodeString(conf.GW.MAC)
	data := make([]byte, 1024)
	for stop == false {
		rlen, err := UDP.Conn.Read(data)
		if err != nil {
			log.Error("faixled to read UDP msg because of ", err)
			continue
		}

		if rlen > 0 {
			plen := rlen
			if plen > 4 {
				plen = 4
			}
			log.Infof("downlink message head: % x", data[:plen])
		}

		if rlen > 12 && data[3] == '\x03' {
			var js map[string]interface{}

			if err := json.Unmarshal(data[4:rlen], &js); err != nil {
				log.Errorf("json parse error: %s\n", err.Error())
				continue
			}

			payloadb64 := js["txpk"].(map[string]interface{})["data"].(string)

			phy := lorawan.PHYPayload{}
			_ = phy.UnmarshalText([]byte(payloadb64))
			if conf.DeviceComm.MACVersion == lorawan.LoRaWAN1_1 {
				phy.DecryptFOpts(nwkSkey)
			}

			//parse maccommand
			macPL, ok := phy.MACPayload.(*lorawan.MACPayload)
			if !ok {
				log.Error("lorawan: MACPayload must be of type *MACPayload\n")
				continue
			}
			if macPL.FPort != nil && *macPL.FPort == 0 {
				phy.DecryptFRMPayload(nwkSkey)
			} else {
				phy.DecryptFRMPayload(appSKey)
			}

			// print phy json
			phystr, _ := json.Marshal(phy)
			log.Infof("downlink phypayload: %s\n", phystr)

			b, _ := b64.StdEncoding.DecodeString(payloadb64)
			js["txpk"].(map[string]interface{})["data"] = fmt.Sprintf("% x", b)

			newpayload, _ := json.Marshal(js)
			log.Infof("downlink macpayload: %s\n", newpayload)

			//answer reply
			reply := append(data[0:4], gweuibytes...)
			reply[3] = '\x05'
			UDP.Send(reply)
			log.Infof("Send downlink reply: % x\n", reply)

			// replay linkadr request
			if macPL.FPort != nil && *macPL.FPort == 0 {
				if strings.Contains(string(phystr), "LinkADRReq") {
					rephy := []byte{'\x03', '\x07', '\x03', '\x07', '\x03', '\x07',
						'\x03', '\x07', '\x03', '\x07', '\x03', '\x07', '\x06', '\xff', '\x06'}
					msg, _ := genPacketBytes(conf, devs[macPL.FHDR.DevAddr], rephy, 0, 0, rephy)
					msg[3] = '\x00'
					log.Infof("LinkAdrAns: %s", msg[12:])
					UDP.Send(msg)
				} else if strings.Contains(string(phystr), "DevStatusReq") {
					rephy := []byte{'\x06', '\xff', '\x0e'}
					msg, _ := genPacketBytes(conf, devs[macPL.FHDR.DevAddr], rephy, 0, 0, nil)
					msg[3] = '\x00'
					log.Infof("DevStatusAns: %s", msg[12:])
					UDP.Send(msg)
				}

			}

			var foptreply []byte
			if strings.Contains(string(phystr), "RXParamSetupReq") {
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

			if strings.Contains(string(phystr), "LinkADRReq") {
				macAns := lorawan.MACCommand{
					CID:     lorawan.LinkADRAns,
					Payload: &lorawan.LinkADRAnsPayload{true, true, true},
				}
				// trcik, do fake adr
				macpayload, _ := macPL.FHDR.FOpts[0].(*lorawan.MACCommand)
				adrpayload, _ := macpayload.Payload.(*lorawan.LinkADRReqPayload)
				conf.DR.SpreadFactor = 12 - int(adrpayload.DataRate)

				rephy, _ := macAns.MarshalBinary()
				foptreply = append(foptreply, rephy...)
			}

			if len(foptreply) > 0 {
				log.Infof("fopt uplink: % x\n", foptreply)
				msg, _ := genPacketBytes(conf, devs[macPL.FHDR.DevAddr], []byte("\x00"), 8, 15, foptreply)
				msg[3] = '\x00'
				log.Infof("MacCommand Ans: %s\n", msg[12:])
				UDP.Send(msg)
			}
		}
	}
}

// send gw state 30s
func sendGWStatelLoop() {
	for stop == false {
		msg := lds.GwStatPacket()
		UDP.Send(msg)
		log.Infof("send gwstat: % x\n", msg)
		time.Sleep(time.Duration(30000) * time.Millisecond)
	}
}

// send gw keepalived 3s
func sendGWKeepalivedLoop() {
	for stop == false {
		msg := lds.GenGWKeepalived()
		UDP.Send(msg)
		log.Infof("send gwkeep: % x\n", msg)
		time.Sleep(time.Duration(3000) * time.Millisecond)
	}
}

func checkTimeoutPackageLoop() {
	for stop == false {
		time.Sleep(time.Duration(conf.DefaultData.Timeout) * time.Millisecond)
		go func() {
			log.Debug("Check timeout package")
			UDP.wg.Add(1)
			defer UDP.wg.Done()

			UDP.SendTimeMutex.Lock()
			defer UDP.SendTimeMutex.Unlock()

			for dk, dv := range UDP.SendTime {
				for k, v := range dv {
					if uint32(time.Now().Sub(v)/time.Millisecond) >= conf.DefaultData.Timeout {
						log.Warningf("msg_timeout[%s] fcnt[%d]", dk, k)
						delete(UDP.SendTime[dk], k)
					}
				}
			}
		}()
	}
}

func (u *udp) init(server string) {

	u.sendChan = make(chan *[]byte, 100)
	u.SendTimeMutex = &sync.Mutex{}

	// Build your node with known keys (ABP).
	nwkSEncHexKey := conf.DeviceComm.NwkSEncKey
	sNwkSIntHexKey := conf.DeviceComm.SNwkSIntKey
	fNwkSIntHexKey := conf.DeviceComm.FNwkSIntKey
	appSHexKey := conf.DeviceComm.AppSKey
	nwkSEncKey, err := lds.HexToKey(nwkSEncHexKey)
	if err != nil {
		log.Errorf("nwkSEncKey error: %s", err)
	}

	sNwkSIntKey, err := lds.HexToKey(sNwkSIntHexKey)
	if err != nil {
		log.Errorf("sNwkSIntKey error: %s", err)
	}

	fNwkSIntKey, err := lds.HexToKey(fNwkSIntHexKey)
	if err != nil {
		log.Errorf("fNwkSIntKey error: %s", err)
	}

	appSKey, err := lds.HexToKey(appSHexKey)
	if err != nil {
		log.Errorf("appskey error: %s", err)
	}

	nwkHexKey := conf.DeviceComm.NwkKey
	appHexKey := conf.DeviceComm.AppKey
	nwkKey, err := lds.HexToKey(nwkHexKey)
	if err != nil {
		log.Errorf("nwkey error: %s", err)
	}
	appKey, err := lds.HexToKey(appHexKey)
	if err != nil {
		log.Errorf("appkey error: %s", err)
	}

	appEUI := [8]byte{0, 0, 0, 0, 0, 0, 0, 0}

	// make(map[uint32]time.time) when add Device
	u.SendTime = make(map[lorawan.EUI64]map[uint32]time.Time)
	devs = make(map[lorawan.DevAddr]*lds.Device)

	for _, dev := range conf.Devices {
		devAddr, err := lds.HexToDevAddress(dev.Address)
		if err != nil {
			log.Errorf("dev addr error: %s", err)
		}
		devEUI, err := lds.HexToEUI(dev.EUI)
		if err != nil {
			return
		}
		u.SendTime[devEUI] = make(map[uint32]time.Time)

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
			Major:       lorawan.Major(conf.DeviceComm.Major),
			MACVersion:  lorawan.MACVersion(conf.DeviceComm.MACVersion),
		}
		device.SetMarshaler(conf.DeviceComm.Marshaler)
		devs[devAddr] = device
	}

	addr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		log.Errorf("Can't resolve address: %s", err)
		os.Exit(1)
	}

	u.Conn, err = net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Error("Can't dial: ", err)
		os.Exit(1)
	}
}

func genPacketBytes(config *config.TomlConfig, d *lds.Device, payload []byte, fport uint8, fOptlen uint8, fOpts []byte) ([]byte, error) {
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
	if config.DeviceComm.MType > 0 {
		mType = lorawan.ConfirmedDataUp
	}

	//Now send an uplink
	msg, err := d.UplinkMessageGWMP(mType, fport, rxInfo, &utx, payload, config.GW.MAC, config.Band.Name, *dataRate, fOptlen, fOpts)

	gwmphead := lds.GenGWMP(config.GW.MAC)
	msg = append(gwmphead[:], msg[:]...)

	UDP.SendTimeMutex.Lock()
	UDP.SendTime[d.DevEUI][d.UlFcnt] = time.Now()
	UDP.SendTimeMutex.Unlock()
	d.UlFcnt++

	return msg, err
}

func deviceRun(dev *lds.Device) {
	mult := 1
	for stop == false {
		payload := []byte{}
		fport := 1
		if conf.RawPayload.UseRaw {
			var pErr error
			payload, pErr = hex.DecodeString(conf.RawPayload.Payload)
			fport = conf.RawPayload.Fport
			if pErr != nil {
				log.Errorf("couldn't decode hex payload: %s\n", pErr)
				return
			}
		} else {
			for _, v := range conf.DefaultData.Data {
				rand.Seed(time.Now().UnixNano() / 10000)
				if rand.Intn(10) < 5 {
					mult *= -1
				}
				num := float32(v[0])
				if conf.DefaultData.Random {
					num = float32(v[0] + float64(mult)*rand.Float64()/100)
				}
				arr := lds.GenerateFloat(num, float32(v[1]), int32(v[2]))
				payload = append(payload, arr...)
			}
		}
		log.Debugf("Send payload, Bytes: % x\n", payload)

		msg, err := genPacketBytes(conf, dev, payload, uint8(fport), 0, nil)
		if err != nil {
			log.Errorf("couldn't generate uplink: %s\n", err)
			return
		}
		log.Infof("Upload message: %v\n", string(msg[12:]))

		UDP.Send(msg)
		time.Sleep(time.Duration(conf.DefaultData.Interval) * time.Millisecond)
	}
}

// RunUDP .. start gateway udp
func RunUDP() {
	// lds gw
	lds.InitGWMP()

	if conf.UDP.Server != "" {
		UDP.init(conf.UDP.Server)
	} else {
		log.Error("udp server is empty")
		os.Exit(-1)
	}
	log.Info("Connection established.")

	loop := []func(){
		readUDPLoop,
		sendUDPLoop,
		sendGWKeepalivedLoop,
		sendGWStatelLoop,
		checkTimeoutPackageLoop,
	}
	for _, fun := range loop {
		go func(f func()) {
			UDP.wg.Add(1)
			defer UDP.wg.Done()
			f()
		}(fun)
	}

	// start device
	for _, dev := range devs {
		go func(dev *lds.Device) {
			UDP.wg.Add(1)
			defer UDP.wg.Done()
			deviceRun(dev)
		}(dev)
	}

	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	log.WithField("signal", <-sigChan).Info("signal received, prepare quit")
	stop = true
	close(UDP.sendChan)
	UDP.wg.Wait()
}
