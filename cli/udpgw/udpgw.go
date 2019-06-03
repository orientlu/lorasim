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
var quit = false

type udp struct {
	Conn              *net.UDPConn
	sendChan          chan *[]byte
	SendTime          map[lorawan.EUI64]map[uint32]time.Time
	SendTimeMutex     *sync.Mutex
	wg                sync.WaitGroup
	devs              map[lorawan.DevAddr]*lds.Device
	spreadFactor      map[lorawan.DevAddr]int
	spreadFactorMutex *sync.RWMutex
}

type stat struct {
	// uplink + ack
	totalSend uint64
	totalRev  uint64

	// uplink + as pub to mqtt
	totalUplink uint64
	totalAsPub  uint64

	//slot
	slotSend   uint64
	slotRev    uint64
	slotUplink uint64
	slotAsPub  uint64

	slot50Ms    uint64
	slot100Ms   uint64
	slot200Ms   uint64
	slot300Ms   uint64
	slot500Ms   uint64
	slot800Ms   uint64
	slot1S      uint64
	slot2S      uint64
	slot5S      uint64
	slot10S     uint64
	slotXXs     uint64
	slotTimeout uint64
}

// STAT ....
var STAT stat

const (
	Slot50ms  = 50
	Slot100ms = Slot50ms * 2
	Slot200ms = Slot100ms * 2
	Slot300ms = Slot100ms * 3
	Slot500ms = Slot100ms * 5
	Slot800ms = Slot100ms * 8
	Slot1s    = Slot500ms * 2
	Slot2s    = Slot1s * 2
	Slot5s    = Slot1s * 5
	Slot10s   = Slot5s * 2
)

// UDP gw udp
var UDP udp

// FunMqttHandle ...
var FunMqttHandle MQTT.MessageHandler = func(client MQTT.Client, msg MQTT.Message) {
	var js map[string]interface{}

	if err := json.Unmarshal(msg.Payload(), &js); err == nil {

		STAT.slotAsPub++

		ackFcnt := (uint32)(js["fCnt"].(float64))
		strEui := js["devEUI"].(string)
		log.WithFields(log.Fields{
			"EUI":  strEui,
			"fcnt": ackFcnt}).Debug("app mqtt msg")

		devEui, err := lds.HexToEUI(strEui)
		if err != nil {
			log.Error("app mqtt HexToEUI error: ", err)
			return
		}
		UDP.SendTimeMutex.Lock()
		defer UDP.SendTimeMutex.Unlock()

		if sendTime, ok := UDP.SendTime[devEui][ackFcnt]; ok {
			fdelay := time.Now().Sub(sendTime) / time.Millisecond
			log.Infof("msg_elapsed[eui:%s]: %d ms\n", strEui, fdelay)
			//log.Infof("msg fcnt[%d] elapsed: %s\n", ackFcnt, time.Since(sendTime))
			delete(UDP.SendTime[devEui], ackFcnt)
			delay := uint64(fdelay)
			switch {
			case delay < Slot50ms:
				STAT.slot50Ms++
			case delay < Slot100ms:
				STAT.slot100Ms++
			case delay < Slot200ms:
				STAT.slot200Ms++
			case delay < Slot300ms:
				STAT.slot300Ms++
			case delay < Slot500ms:
				STAT.slot500Ms++
			case delay < Slot800ms:
				STAT.slot800Ms++
			case delay < Slot1s:
				STAT.slot1S++
			case delay < Slot2s:
				STAT.slot2S++
			case delay < Slot5s:
				STAT.slot5S++
			case delay < Slot10s:
				STAT.slot10S++
			default:
				STAT.slotXXs++
			}

		} else {
			log.Infof("app mqtt msg fnct[%d] can not found", ackFcnt)
		}

	} else {
		log.Errorf("app mqtt msg json unmarshal error %s", err)
	}
}

func (u *udp) Send(msg []byte) {
	UDP.sendChan <- &msg
	log.Trace("udp package push to Channel: ", msg)
}

func sendUDPLoop() {
	for msg := range UDP.sendChan {
		_, err := UDP.Conn.Write(*msg)
		if err != nil {
			log.Error("udp send failed: ", err)
			continue
		}
		STAT.slotSend++

		log.Trace("udp send package: ", msg)
	}
	// close udp when quit
	UDP.Conn.Close()
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
	for quit == false {
		rlen, err := UDP.Conn.Read(data)
		if err != nil {
			log.Error("failed to read UDP msg because of ", err)
			continue
		}
		STAT.slotRev++

		if rlen > 0 {
			plen := rlen
			if plen > 4 {
				plen = 4
			}
			log.Debugf("downlink message head: % x", data[:plen])
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
			log.Debugf("downlink phypayload: %s\n", phystr)

			b, _ := b64.StdEncoding.DecodeString(payloadb64)
			js["txpk"].(map[string]interface{})["data"] = fmt.Sprintf("% x", b)

			newpayload, _ := json.Marshal(js)
			log.Debugf("downlink macpayload: %s\n", newpayload)

			//answer reply
			reply := make([]byte, 4)
			copy(reply, data[0:4])
			reply = append(reply, gweuibytes...)
			reply[3] = '\x05'
			UDP.Send(reply)
			log.Debugf("Send downlink reply: % x\n", reply)

			// replay linkadr request
			if macPL.FPort != nil && *macPL.FPort == 0 {
				if strings.Contains(string(phystr), "LinkADRReq") {
					rephy := []byte{'\x03', '\x07', '\x03', '\x07', '\x03', '\x07',
						'\x03', '\x07', '\x03', '\x07', '\x03', '\x07', '\x06', '\xff', '\x06'}
					msg, _ := genPacketBytes(conf, UDP.devs[macPL.FHDR.DevAddr], rephy, 0, 0, rephy)
					msg[3] = '\x00'
					log.Debugf("LinkAdrAns: %s", msg[12:])
					UDP.Send(msg)
				} else if strings.Contains(string(phystr), "DevStatusReq") {
					rephy := []byte{'\x06', '\xff', '\x0e'}
					msg, _ := genPacketBytes(conf, UDP.devs[macPL.FHDR.DevAddr], rephy, 0, 0, nil)
					msg[3] = '\x00'
					log.Debugf("DevStatusAns: %s", msg[12:])
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
				UDP.spreadFactorMutex.Lock()
				UDP.spreadFactor[macPL.FHDR.DevAddr] = 12 - int(adrpayload.DataRate)
				UDP.spreadFactorMutex.Unlock()

				rephy, _ := macAns.MarshalBinary()
				foptreply = append(foptreply, rephy...)
			}

			if len(foptreply) > 0 {
				log.Debugf("fopt uplink: % x\n", foptreply)
				msg, _ := genPacketBytes(conf, UDP.devs[macPL.FHDR.DevAddr], []byte("\x00"), 8, 15, foptreply)
				msg[3] = '\x00'
				log.Debugf("MacCommand Ans: %s\n", msg[12:])
				UDP.Send(msg)
			}
		}
	}
}

// send gw state 30s
func sendGWStatelLoop() {
	for quit == false {
		msg := lds.GwStatPacket()
		UDP.Send(msg)
		log.Debugf("send gwstat: % x\n", msg)
		time.Sleep(time.Duration(30000) * time.Millisecond)
	}
}

// send gw keepalived 3s
func sendGWKeepalivedLoop() {
	for quit == false {
		msg := lds.GenGWKeepalived()
		UDP.Send(msg)
		log.Debugf("send gwkeep: % x\n", msg)
		time.Sleep(time.Duration(3000) * time.Millisecond)
	}
}

func checkTimeoutPackageLoop() {
	for quit == false {
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

						STAT.slotTimeout++
					}
				}
			}
		}()
	}
}

func statLoop() {
	for quit == false {
		time.Sleep(time.Duration(conf.DefaultData.StatInterval) * time.Millisecond)

		STAT.totalRev += STAT.slotRev
		STAT.totalAsPub += STAT.slotAsPub
		STAT.totalSend += STAT.slotSend
		STAT.totalUplink += STAT.slotUplink

		log.WithFields(log.Fields{
			"Send":        STAT.slotSend,
			"Rev":         STAT.slotRev,
			"Uplink":      STAT.slotUplink,
			"AsPub":       STAT.slotAsPub,
			"TotalUplink": STAT.totalUplink,
			"TotalAsPub":  STAT.totalAsPub,
			"TotalSend":   STAT.totalSend,
			"TotalRev":    STAT.totalRev,
		}).Warning("stat(package):")

		f50ms := (float64)(STAT.slot50Ms) * 100.0 / (float64)(STAT.slotAsPub)
		f100ms := (float64)(STAT.slot100Ms) * 100.0 / (float64)(STAT.slotAsPub)
		f200ms := (float64)(STAT.slot200Ms) * 100.0 / (float64)(STAT.slotAsPub)
		f300ms := (float64)(STAT.slot300Ms) * 100.0 / (float64)(STAT.slotAsPub)
		f500ms := (float64)(STAT.slot500Ms) * 100.0 / (float64)(STAT.slotAsPub)
		f800ms := (float64)(STAT.slot800Ms) * 100.0 / (float64)(STAT.slotAsPub)
		f1s := (float64)(STAT.slot1S) * 100.0 / (float64)(STAT.slotAsPub)
		f2s := (float64)(STAT.slot2S) * 100.0 / (float64)(STAT.slotAsPub)
		f5s := (float64)(STAT.slot5S) * 100.0 / (float64)(STAT.slotAsPub)
		f10s := (float64)(STAT.slot10S) * 100.0 / (float64)(STAT.slotAsPub)
		fxxs := (float64)(STAT.slotXXs) * 100.0 / (float64)(STAT.slotAsPub)

		log.WithFields(log.Fields{
			"50ms":    f50ms,
			"100ms":   f100ms,
			"200ms":   f200ms,
			"300ms":   f300ms,
			"500ms":   f500ms,
			"800ms":   f800ms,
			"1s":      f1s,
			"2s":      f2s,
			"5s":      f5s,
			"10s":     f10s,
			"more":    fxxs,
			"timeout": STAT.slotTimeout,
		}).Warning("stat(elapsed distribution %):")

		STAT.slot50Ms = 0
		STAT.slot100Ms = 0
		STAT.slot200Ms = 0
		STAT.slot300Ms = 0
		STAT.slot500Ms = 0
		STAT.slot800Ms = 0
		STAT.slot1S = 0
		STAT.slot2S = 0
		STAT.slot5S = 0
		STAT.slot10S = 0
		STAT.slotXXs = 0
		STAT.slotTimeout = 0

		STAT.slotRev = 0
		STAT.slotAsPub = 0
		STAT.slotSend = 0
		STAT.slotUplink = 0

	}
}

func (u *udp) init(server string) (err error) {

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
		return
	}

	sNwkSIntKey, err := lds.HexToKey(sNwkSIntHexKey)
	if err != nil {
		log.Errorf("sNwkSIntKey error: %s", err)
		return
	}

	fNwkSIntKey, err := lds.HexToKey(fNwkSIntHexKey)
	if err != nil {
		log.Errorf("fNwkSIntKey error: %s", err)
		return
	}

	appSKey, err := lds.HexToKey(appSHexKey)
	if err != nil {
		log.Errorf("appSKey error: %s", err)
		return
	}

	nwkHexKey := conf.DeviceComm.NwkKey
	appHexKey := conf.DeviceComm.AppKey
	nwkKey, err := lds.HexToKey(nwkHexKey)
	if err != nil {
		log.Errorf("nwkKey error: %s", err)
		return
	}
	appKey, err := lds.HexToKey(appHexKey)
	if err != nil {
		log.Errorf("appKey error: %s", err)
		return
	}

	appEUI := [8]byte{6, 0, 0, 0, 0, 0, 0, 0}

	// make(map[uint32]time.time) when add Device
	u.SendTime = make(map[lorawan.EUI64]map[uint32]time.Time)
	u.devs = make(map[lorawan.DevAddr]*lds.Device)
	u.spreadFactor = make(map[lorawan.DevAddr]int)
	u.spreadFactorMutex = &sync.RWMutex{}

	for _, dev := range conf.Devices {
		log.WithField("EUI", dev.EUI).Debug("Init device")
		devAddr, err := lds.HexToDevAddress(dev.Address)
		if err != nil {
			log.Errorf("dev addr error: %s", err)
			return err
		}
		devEUI, err := lds.HexToEUI(dev.EUI)
		if err != nil {
			return err
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
		u.devs[devAddr] = device
		u.spreadFactor[devAddr] = conf.DR.SpreadFactor
	}

	addr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		log.Errorf("Can't resolve address: %s", err)
		return err
	}

	u.Conn, err = net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Error("Can't dial: ", err)
		return err
	}

	return
}

func genPacketBytes(config *config.TomlConfig, d *lds.Device, payload []byte, fport uint8, fOptlen uint8, fOpts []byte) ([]byte, error) {
	UDP.spreadFactorMutex.RLock()
	dataRate := &lds.DataRate{
		Bandwidth:    config.DR.Bandwith,
		Modulation:   "LORA",
		SpreadFactor: UDP.spreadFactor[d.DevAddr],
		BitRate:      config.DR.BitRate,
	}
	UDP.spreadFactorMutex.RUnlock()

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

	STAT.slotUplink++

	return msg, err
}

func deviceRun(dev *lds.Device) {
	mult := 1

	// disperse device slot
	rand.Seed(time.Now().UnixNano())
	delay := rand.Intn(int(conf.DefaultData.Interval))
	time.Sleep(time.Duration(delay) * time.Millisecond)
	log.Infof("Device[eui:%s]: after delay %d ms", dev.DevEUI, delay)

	for quit == false {
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
		log.Tracef("Send payload, Bytes: % x\n", payload)

		msg, err := genPacketBytes(conf, dev, payload, uint8(fport), 0, nil)
		if err != nil {
			log.Errorf("couldn't generate uplink: %s\n", err)
			return
		}
		log.Debugf("Upload message: %v\n", string(msg[12:]))

		UDP.Send(msg)
		time.Sleep(time.Duration(conf.DefaultData.Interval) * time.Millisecond)
	}
}

// RunUDP .. start gateway udp
func RunUDP() {
	// lds gw
	lds.InitGWMP()

	if conf.UDP.Server != "" {
		err := UDP.init(conf.UDP.Server)
		if err != nil {
			log.Errorf("UDP server init error: %s", err)
			return
		}
	} else {
		log.Error("udp server is empty")
		return
	}
	log.Debug("Connection established.")

	loop := []func(){
		readUDPLoop,
		sendUDPLoop,
		sendGWKeepalivedLoop,
		sendGWStatelLoop,
		checkTimeoutPackageLoop,
		statLoop,
	}
	for _, fun := range loop {
		go func(f func()) {
			UDP.wg.Add(1)
			defer UDP.wg.Done()
			f()
		}(fun)
	}

	// start device
	for _, dev := range UDP.devs {
		go func(dev *lds.Device) {
			UDP.wg.Add(1)
			defer UDP.wg.Done()
			deviceRun(dev)
		}(dev)
	}

	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	log.WithField("signal", <-sigChan).Info("signal received, prepare quit")

	exitChan := make(chan struct{})
	go func() {
		log.Warning("Close UDP and stopping all devices, please wait a minute...")
		quit = true
		close(UDP.sendChan)
		UDP.wg.Wait()
		exitChan <- struct{}{}
	}()
	select {
	case <-exitChan: // wait
	case s := <-sigChan:
		log.WithField("signal", s).Warning("signal received again, stopping immediately")
	}
}
