package main

import (
	"net/http"
	_ "net/http/pprof"

	"flag"
	"fmt"
	"strings"
	"time"

	"git.code.oa.com/orientlu/lorasim/lds"

	"github.com/brocaar/lorawan"
	lwband "github.com/brocaar/lorawan/band"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type mqtt struct {
	Server   string `mapstructure:"server"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Client   MQTT.Client
}

type gateway struct {
	MAC string `mapstructure:"mac"`
}

type band struct {
	Name lwband.Name `mapstructure:"name"`
}

type device struct {
	EUI         string             `mapstructure:"eui"`
	Address     string             `mapstructure:"address"`
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

type tomlConfig struct {
	MQTT        mqtt        `mapstructure:"mqtt"`
	UDP         udp         `mapstructure:"udp"`
	Band        band        `mapstructure:"band"`
	Device      device      `timl:"device"`
	GW          gateway     `mapstructure:"gateway"`
	DR          dataRate    `mapstructure:"data_rate"`
	RXInfo      rxInfo      `mapstructure:"rx_info"`
	DefaultData defaultData `mapstructure:"default_data"`
	RawPayload  rawPayload  `mapstructure:"raw_payload"`
}

var confFile *string
var config tomlConfig
var stop bool
var marshalers = map[string]int{"json": 0, "protobuf": 1, "v2_json": 2}
var bands = []lwband.Name{
	lwband.AS_923,
	lwband.AU_915_928,
	lwband.CN_470_510,
	lwband.CN_779_787,
	lwband.EU_433,
	lwband.EU_863_870,
	lwband.IN_865_867,
	lwband.KR_920_923,
	lwband.US_902_928,
	lwband.RU_864_870,
}
var sendOnce bool
var interval int

func importConf() {
	viper.SetConfigFile(*confFile)
	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		switch err.(type) {
		case viper.ConfigFileNotFoundError:
			log.Warning("No configuration file found, using default.")
		default:
			log.WithError(err).Fatal("read configuration file error")
		}
	} else {
		log.Println("Using config file:", viper.ConfigFileUsed())
	}

	// read in environment variables that match
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.Unmarshal(&config); err != nil {
		log.WithError(err).Fatal("unmarshal config error")
	}
}

func setupMqtt(config *tomlConfig) {

	opts := MQTT.NewClientOptions().AddBroker(config.MQTT.Server)
	opts.SetKeepAlive(2 * time.Second)
	opts.SetPingTimeout(1 * time.Second)
	opts.SetUsername(config.MQTT.User)
	opts.SetPassword(config.MQTT.Password)
	opts.SetCleanSession(true)
	opts.SetClientID(fmt.Sprintf("sim-%s", config.Device.EUI))
	opts.SetOnConnectHandler(onMqttConnected)
	opts.SetConnectionLostHandler(func(c MQTT.Client, reason error) {
		log.Errorf("Mqtt lost connect %s", reason)
	})
	config.MQTT.Client = MQTT.NewClient(opts)
	for {
		if token := config.MQTT.Client.Connect(); token.Wait() && token.Error() != nil {
			log.Errorf("mqtt: connecting to mqtt broker [%s] failed, will retry in 2s: %s", config.MQTT.Server, token.Error())
			time.Sleep(2 * time.Second)
		} else {
			break
		}
	}
}

func onMqttConnected(c MQTT.Client) {
	topic := fmt.Sprintf("application/%s/device/%s/rx", config.Device.Application, config.Device.EUI)
	for {
		log.WithFields(log.Fields{
			"topic": topic,
			"qos":   0,
		}).Info("Mqtt: subscribing topic")
		if token := c.Subscribe(topic, 0, FunMqttHandle); token.Wait() && token.Error() != nil {
			log.Error(token.Error(), "retry 1 second")
			time.Sleep(time.Second)
			continue
		}
		break
	}
}

func setupPprof(isStart bool) {
	if isStart {
		log.Println(http.ListenAndServe("0.0.0.0:6060", nil))
	}
}

func main() {
	confFile = flag.String("conf", "conf.toml", "path to toml configuration file")
	logpath := flag.Bool("logpath", false, "log call path")
	pprof := flag.Bool("pprof", false, "run pprof server")

	flag.Parse()
	log.SetReportCaller(*logpath)

	importConf()
	lds.GwEUI = config.GW.MAC

	go setupPprof(*pprof)

	setupMqtt(&config)

	RunUdp(&config)
}
