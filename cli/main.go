package main

import (
	"net/http"
	_ "net/http/pprof"

	"flag"
	"fmt"

	"git.code.oa.com/orientlu/lorasim/cli/config"
	"git.code.oa.com/orientlu/lorasim/cli/tracing"
	"git.code.oa.com/orientlu/lorasim/cli/udpgw"
	"git.code.oa.com/orientlu/lorasim/lds"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"strings"
	"time"
)

var confFile *string
var quit = false

// read config from flag/env/file
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
		log.Debug("Using config file:", viper.ConfigFileUsed())
	}

	// read in environment variables that match
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.Unmarshal(&config.C); err != nil {
		log.WithError(err).Fatal("unmarshal config error")
	}
}

/// subscribe AS topic, for stat
func setupMqtt(config *config.TomlConfig) {

	opts := MQTT.NewClientOptions().AddBroker(config.MQTT.Server)
	opts.SetKeepAlive(2 * time.Second)
	opts.SetPingTimeout(1 * time.Second)
	opts.SetUsername(config.MQTT.User)
	opts.SetPassword(config.MQTT.Password)
	opts.SetCleanSession(true)
	opts.SetClientID(fmt.Sprintf("lorasim-gw-%s", config.GW.MAC))
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
	for _, dev := range config.C.Devices {
		EUI := dev.EUI
		go func(EUI string) {
			// AS publish topic
			topic := fmt.Sprintf("application/%s/device/%s/rx", config.C.DeviceComm.Application, EUI)
			for {
				if quit {
					break
				}
				log.WithFields(log.Fields{
					"topic": topic,
					"qos":   0,
				}).Debug("Mqtt: subscribing topic")
				if token := c.Subscribe(topic, 0, udpgw.FunMqttHandle); token.Wait() && token.Error() != nil {
					log.Error(token.Error(), "retry 1 second")
					time.Sleep(time.Second)
					continue
				}
				break
			}
		}(EUI)
	}
}

// run pprof
func setupPprof(isStart bool) {
	if isStart {
		log.Println(http.ListenAndServe("0.0.0.0:6060", nil))
	}
}

func main() {
	tracing.InitTracing("")
	confFile = flag.String("conf", "conf.toml", "path to toml configuration file")
	logLevel := flag.Uint("loglevel", 4, "log level, default is 4; debug=5, info=4..")
	showlogpath := flag.Bool("showlogpath", false, "log call path")
	pprof := flag.Bool("pprof", false, "run pprof server")

	flag.Parse()
	log.SetReportCaller(*showlogpath)
	log.SetLevel(log.Level(*logLevel))
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})

	importConf()
	lds.GwEUI = config.C.GW.MAC

	go setupPprof(*pprof)

	setupMqtt(&config.C)

	udpgw.RunUDP()

	quit = true
	config.C.MQTT.Client.Disconnect(200)

	tracing.DeInitTracing()
}
