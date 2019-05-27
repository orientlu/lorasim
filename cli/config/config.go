package config

import (
	"github.com/brocaar/lorawan"
	lwband "github.com/brocaar/lorawan/band"
	MQTT "github.com/eclipse/paho.mqtt.golang"
)

// loracli acts as a gateway which forward multdevices package
// acts as semte gw, use udp with gateway bridge

type udp struct {
	Server string `mapstructure:"server"`
}

// receive AS topic: application/app_id/device/dev_eui/rx
type mqtt struct {
	Server   string `mapstructure:"server"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`

	Client MQTT.Client
}

type gateway struct {
	MAC string `mapstructure:"mac"`
}

type band struct {
	Name lwband.Name `mapstructure:"name"`
}

// all devices share the common config
type deviceCommon struct {
	NwkSEncKey  string             `mapstructure:"network_session_encription_key"`
	SNwkSIntKey string             `mapstructure:"serving_network_session_integrity_key"`    //For Lorawan 1.0 this is the same as the NwkSEncKey
	FNwkSIntKey string             `mapstructure:"forwarding_network_session_integrity_key"` //For Lorawan 1.0 this is the same as the NwkSEncKey
	AppSKey     string             `mapstructure:"application_session_key"`
	Marshaler   string             `mapstructure:"marshaler"`
	NwkKey      string             `mapstructure:"nwk_key"`     //Network key, used to be called application key for Lorawan 1.0
	AppKey      string             `mapstructure:"app_key"`     //Application key, for Lorawan 1.1
	Major       lorawan.Major      `mapstructure:"major"`       //Lorawan major version
	MACVersion  lorawan.MACVersion `mapstructure:"mac_version"` //Lorawan MAC version
	MType       lorawan.MType      `mapstructure:"mtype"`       //LoRaWAN mtype (ConfirmedDataUp or UnconfirmedDataUp)
	Application string             `mapstructure:"application"` // owner application
}

type device struct {
	EUI     string `mapstructure:"eui"`
	Address string `mapstructure:"address"`
}

type dataRate struct {
	Bandwith     int `mapstructure:"bandwith"`
	SpreadFactor int `mapstructure:"spread_factor"`
	BitRate      int `mapstructure:"bit_rate"`
}

type rxInfo struct {
	Channel   int     `mapstructure:"channel"`
	CodeRate  string  `mapstructure:"code_rate"`
	CrcStatus int     `mapstructure:"crc_status"`
	Frequency int     `mapstructure:"frequency"`
	LoRaSNR   float64 `mapstructure:"lora_snr"`
	RfChain   int     `mapstructure:"rf_chain"`
	Rssi      int     `mapstructure:"rssi"`
}

//defaultData holds optional default encoded data.
type defaultData struct {
	Names    []string    `mapstructure:"names"`
	Data     [][]float64 `mapstructure:"data"`
	Interval int32       `mapstructure:"interval"`
	Random   bool        `mapstructure:"random"`
	Timeout  uint32      `mapstructure:"timeout"`
}

//rawPayload holds optional raw bytes payload (hex encoded).
type rawPayload struct {
	Payload string `mapstructure:"payload"`
	UseRaw  bool   `mapstructure:"use_raw"`
	Fport   int    `mapstructure:"fport"`
}

// TomlConfig is lorasim config struct
type TomlConfig struct {
	MQTT        mqtt         `mapstructure:"mqtt"`
	UDP         udp          `mapstructure:"udp"`
	Band        band         `mapstructure:"band"`
	DeviceComm  deviceCommon `mapstructure:"device_common"`
	Devices     []device     `mapstructure:"devices"`
	GW          gateway      `mapstructure:"gateway"`
	DR          dataRate     `mapstructure:"data_rate"`
	RXInfo      rxInfo       `mapstructure:"rx_info"`
	DefaultData defaultData  `mapstructure:"default_data"`
	RawPayload  rawPayload   `mapstructure:"raw_payload"`
}

// C is all config
var C TomlConfig
