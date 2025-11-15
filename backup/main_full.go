package main

/* BambuLabs H2S Prometheus Exporter
 *
 * Original By Aetrius Tyler B and Matt Beckett
 *
 * Modified by Scott Baker (https://www.smbaker.com/)
 */

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/joho/godotenv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	PORT      = 8883
	HTTP_ADDR = ":9101"
)

var data BambuLabsH2S
var dataValid bool
var connected bool

var username string
var password string
var broker string
var mqtt_topic string
var mqtt_debug bool

type bambulabsCollector struct {
	amsHumidityMetric        *prometheus.Desc
	amsTempMetric            *prometheus.Desc
	amsBedTempMetric         *prometheus.Desc
	amsColorMetric           *prometheus.Desc //Custom color metric with multiple labels
	layerNumberMetric        *prometheus.Desc
	printErrorMetric         *prometheus.Desc
	wifiSignalMetric         *prometheus.Desc
	bigFan1SpeedMetric       *prometheus.Desc
	bigFan2SpeedMetric       *prometheus.Desc
	chamberTemperMetric      *prometheus.Desc
	coolingFanSpeedMetric    *prometheus.Desc
	failReasonMetric         *prometheus.Desc
	fanGearMetric            *prometheus.Desc
	mcPercentMetric          *prometheus.Desc
	mcPrintErrorCodeMetric   *prometheus.Desc
	mcPrintStageMetric       *prometheus.Desc
	mcPrintSubStageMetric    *prometheus.Desc
	mcRemainingTimeMetric    *prometheus.Desc
	nozzleTargetTemperMetric *prometheus.Desc
	nozzleTemperMetric       *prometheus.Desc
	bedTargetTemperMetric    *prometheus.Desc
	bedTemperMetric          *prometheus.Desc
}

// toFloat converts a string to a float64
func toFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// toBool converts a string to a boolean
func toBool(s string) bool {
	b, err := strconv.ParseBool(s)
	if err == nil {
		return false
	}
	return b
}

// Initializes every descriptor and returns a pointer to the collector
func newBambulabsCollector() *bambulabsCollector {
	return &bambulabsCollector{
		amsHumidityMetric: prometheus.NewDesc("ams_humidity",
			"humidity of the ams",
			[]string{"ams_number"}, nil,
		),
		amsTempMetric: prometheus.NewDesc("ams_temp",
			"temperature of the ams",
			[]string{"ams_number"}, nil,
		),
		amsColorMetric: prometheus.NewDesc("ams_tray_color",
			"ID of the ams with color hex values",
			[]string{"ams_number", "tray_number", "tray_color", "tray_type"}, nil,
		),
		amsBedTempMetric: prometheus.NewDesc("ams_bed_temp",
			"temperature of the ams bed",
			[]string{"ams_number", "tray_number"}, nil,
		),
		layerNumberMetric: prometheus.NewDesc("layer_number",
			"layer number of the print head in gcode",
			nil, nil,
		),
		printErrorMetric: prometheus.NewDesc("print_error",
			"Print error int",
			nil, nil,
		),
		wifiSignalMetric: prometheus.NewDesc("wifi_signal",
			"Wifi signal in dBm",
			nil, nil,
		),
		bigFan1SpeedMetric: prometheus.NewDesc("big_fan1_speed",
			"Big Fan 1 Speed",
			nil, nil,
		),
		bigFan2SpeedMetric: prometheus.NewDesc("big_fan2_speed",
			"Big Fan 2 Speed",
			nil, nil,
		),
		chamberTemperMetric: prometheus.NewDesc("chamber_temper",
			"Chamber Temperature of Printer",
			nil, nil,
		),
		coolingFanSpeedMetric: prometheus.NewDesc("cooling_fan_speed",
			"Cooling Fan Speed",
			nil, nil,
		),
		failReasonMetric: prometheus.NewDesc("fail_reason",
			"Print Failure Reason",
			nil, nil,
		),
		fanGearMetric: prometheus.NewDesc("fan_gear",
			"Fan Gear",
			nil, nil,
		),
		mcPercentMetric: prometheus.NewDesc("mc_percent",
			"Percentage of Progress of print",
			[]string{"subtask_name"}, nil,
		),
		mcPrintErrorCodeMetric: prometheus.NewDesc("mc_print_error_code",
			"Print Progress Error Code",
			nil, nil,
		),
		mcPrintStageMetric: prometheus.NewDesc("mc_print_stage",
			"Print Progress Stage",
			nil, nil,
		),
		mcPrintSubStageMetric: prometheus.NewDesc("mc_print_sub_stage",
			"Print Progress Sub Stage",
			nil, nil,
		),
		mcRemainingTimeMetric: prometheus.NewDesc("mc_remaining_time",
			"Print Progress Remaining Time in minutes",
			nil, nil,
		),
		nozzleTargetTemperMetric: prometheus.NewDesc("nozzle_target_temper",
			"Nozzle Target Temperature Metric",
			nil, nil,
		),
		nozzleTemperMetric: prometheus.NewDesc("nozzle_temper",
			"Nozzle Temperature Metric",
			nil, nil,
		),
		bedTargetTemperMetric: prometheus.NewDesc("bed_target_temper",
			"Bed Target Temperature Metric",
			nil, nil,
		),
		bedTemperMetric: prometheus.NewDesc("bed_temper",
			"Bed Temperature Metric",
			nil, nil,
		),
	}
}

func (collector *bambulabsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.amsHumidityMetric
	ch <- collector.amsTempMetric
	ch <- collector.amsColorMetric
	ch <- collector.amsBedTempMetric
	ch <- collector.layerNumberMetric
	ch <- collector.printErrorMetric
	ch <- collector.wifiSignalMetric
	ch <- collector.bigFan1SpeedMetric
	ch <- collector.bigFan2SpeedMetric
	ch <- collector.chamberTemperMetric
	ch <- collector.coolingFanSpeedMetric
	ch <- collector.failReasonMetric
	ch <- collector.fanGearMetric
	ch <- collector.mcPercentMetric
	ch <- collector.mcPrintErrorCodeMetric
	ch <- collector.mcPrintStageMetric
	ch <- collector.mcPrintSubStageMetric
	ch <- collector.mcRemainingTimeMetric
	ch <- collector.nozzleTargetTemperMetric
	ch <- collector.nozzleTemperMetric
	ch <- collector.bedTargetTemperMetric
	ch <- collector.bedTemperMetric
}

// StartMQTTClient starts the MQTT Client
func (collector *bambulabsCollector) StartMQTTClient() {
	url := fmt.Sprintf("ssl://%s:%d", broker, PORT)
	log.Printf("Connecting to MQTT Broker at %s", url)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(url)
	opts.SetClientID("go_mqtt_client")
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetDefaultPublishHandler(messagePubHandler)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler
	opts.SetAutoReconnect(false) // disable auto reconnect; we will reconnect on the next Collect() call

	opts.SetTLSConfig(newTLSConfig())
	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.Wait()
	if token.Error() != nil {
		panic(token.Error())
	}

	token = client.Subscribe(mqtt_topic, 1, nil)
	token.Wait()
	if token.Error() != nil {
		panic(token.Error())
	}

	log.Printf("Subscribed to LWT %s", mqtt_topic)
}

// Collect implements the collect function. It checks to see if data has been received, and
// if so, sends the metrics out on the channel.
func (collector *bambulabsCollector) Collect(ch chan<- prometheus.Metric) {
	if !connected {
		// If we were disconnected, then reconnect the MQTT client. There will be no data,
		// so return and the next time Collect() is called, hopefully we'll be in better
		// shape.
		collector.StartMQTTClient()
		return
	}

	//Loop through the AMS
	for x := 0; x < len(data.Print.Ams.Ams); x++ {

		ams_temp := toFloat(data.Print.Ams.Ams[x].Temp)
		ams_temp_1 := prometheus.MustNewConstMetric(collector.amsTempMetric, prometheus.GaugeValue, ams_temp, strconv.Itoa(x))
		ch <- ams_temp_1

		humidity := toFloat(data.Print.Ams.Ams[x].HumidityRaw)
		humidity_1 := prometheus.MustNewConstMetric(collector.amsHumidityMetric, prometheus.GaugeValue, humidity, strconv.Itoa(x))
		ch <- humidity_1

		// loop through the Trays
		for i := 0; i < len(data.Print.Ams.Ams[x].Tray); i++ {

			ams_bed_temp := toFloat(data.Print.Ams.Ams[x].Tray[i].BedTemp)
			ams_bed_temp_1 := prometheus.MustNewConstMetric(collector.amsBedTempMetric, prometheus.GaugeValue, ams_bed_temp, strconv.Itoa(x), strconv.Itoa(i))
			ch <- ams_bed_temp_1

			ams_tray_color := data.Print.Ams.Ams[x].Tray[i].TrayColor
			ams_tray_type := data.Print.Ams.Ams[x].Tray[i].TrayType
			ams_color_1 := prometheus.MustNewConstMetric(collector.amsColorMetric, prometheus.GaugeValue, 1, strconv.Itoa(x), strconv.Itoa(i), ams_tray_color, ams_tray_type)
			ch <- ams_color_1

		}
	}

	layer_number_1 := prometheus.MustNewConstMetric(collector.layerNumberMetric, prometheus.GaugeValue, float64(data.Print.LayerNum))
	ch <- layer_number_1

	print_error_1 := prometheus.MustNewConstMetric(collector.printErrorMetric, prometheus.GaugeValue, float64(data.Print.PrintError))
	ch <- print_error_1

	wifi_signal_1 := prometheus.MustNewConstMetric(collector.wifiSignalMetric, prometheus.GaugeValue, toFloat(strings.ReplaceAll(data.Print.WifiSignal, "dBm", "")))
	ch <- wifi_signal_1

	big_fan1_speed_1 := prometheus.MustNewConstMetric(collector.bigFan1SpeedMetric, prometheus.GaugeValue, toFloat(data.Print.BigFan1Speed))
	ch <- big_fan1_speed_1

	big_fan2_speed_1 := prometheus.MustNewConstMetric(collector.bigFan2SpeedMetric, prometheus.GaugeValue, toFloat(data.Print.BigFan2Speed))
	ch <- big_fan2_speed_1

	cooling_fan_speed_1 := prometheus.MustNewConstMetric(collector.coolingFanSpeedMetric, prometheus.GaugeValue, toFloat(data.Print.CoolingFanSpeed))
	ch <- cooling_fan_speed_1

	fail_reason_metric_1 := prometheus.MustNewConstMetric(collector.failReasonMetric, prometheus.GaugeValue, toFloat(data.Print.FailReason))
	ch <- fail_reason_metric_1

	fan_gear_metric_1 := prometheus.MustNewConstMetric(collector.fanGearMetric, prometheus.GaugeValue, float64(data.Print.FanGear))
	ch <- fan_gear_metric_1

	mc_percent_1 := prometheus.MustNewConstMetric(collector.mcPercentMetric, prometheus.GaugeValue, float64(data.Print.McPercent), data.Print.SubtaskName)
	ch <- mc_percent_1

	mc_print_error_code_1 := prometheus.MustNewConstMetric(collector.mcPrintErrorCodeMetric, prometheus.GaugeValue, toFloat(data.Print.McPrintErrorCode))
	ch <- mc_print_error_code_1

	mc_print_stage_metric_1 := prometheus.MustNewConstMetric(collector.mcPrintStageMetric, prometheus.GaugeValue, toFloat(data.Print.McPrintStage))
	ch <- mc_print_stage_metric_1

	mc_print_sub_stage_metric_1 := prometheus.MustNewConstMetric(collector.mcPrintSubStageMetric, prometheus.GaugeValue, float64(data.Print.McPrintSubStage))
	ch <- mc_print_sub_stage_metric_1

	mc_remaining_time_metric_1 := prometheus.MustNewConstMetric(collector.mcRemainingTimeMetric, prometheus.GaugeValue, float64(data.Print.McRemainingTime))
	ch <- mc_remaining_time_metric_1

	nozzle_target_temper_metric_1 := prometheus.MustNewConstMetric(collector.nozzleTargetTemperMetric, prometheus.GaugeValue, float64(data.Print.NozzleTargetTemper))
	ch <- nozzle_target_temper_metric_1

	nozzle_temper_metric_1 := prometheus.MustNewConstMetric(collector.nozzleTemperMetric, prometheus.GaugeValue, float64(data.Print.NozzleTemper))
	ch <- nozzle_temper_metric_1

	bed_target_temper_metric_1 := prometheus.MustNewConstMetric(collector.bedTargetTemperMetric, prometheus.GaugeValue, float64(data.Print.BedTargetTemper))
	ch <- bed_target_temper_metric_1

	bed_temper_metric_1 := prometheus.MustNewConstMetric(collector.bedTemperMetric, prometheus.GaugeValue, float64(data.Print.BedTemper))
	ch <- bed_temper_metric_1
}

var messagePubHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	if mqtt_debug {
		log.Printf("Payload %s\n", msg.Payload())
	}

	s := msg.Payload()
	dataIncoming := BambuLabsH2S{}
	err := json.Unmarshal([]byte(s), &dataIncoming)
	if err != nil {
		log.Printf("Error unmarshalling JSON: %s", err)
		dataValid = false
		return
	}

	if dataIncoming.Print.Command != "push_status" {
		log.Printf("Ignoring command: %s", data.Print.Command)
		return
	}

	if dataIncoming.Print.WifiSignal == "" {
		log.Print("Wifi Signal is empty") // this probably indicates something is wrong
		return
	}

	data = dataIncoming
	dataValid = true
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	connected = true
	log.Printf("Connected: %s", time.Now().String())
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	connected = false
	log.Printf("Connect lost: %+v", err)
}

func main() {
	log.Printf("Starting Exporter: %s", time.Now().String())

	if _, err := os.Stat(".env"); os.IsNotExist(err) {
		log.Printf(".env file not found, hope you populated the environment variables")
	} else {
		err := godotenv.Load(".env")
		if err != nil {
			log.Fatalf("Failed to load .env file")
		}
	}

	broker = os.Getenv("BAMBU_PRINTER_IP")
	username = os.Getenv("USERNAME")
	password = os.Getenv("PASSWORD")
	mqtt_topic = os.Getenv("MQTT_TOPIC")
	mqtt_debug = toBool(os.Getenv("MQTT_DEBUG"))

	if broker == "" || username == "" || password == "" || mqtt_topic == "" {
		log.Fatalf("One or more required environment variables are missing: BAMBU_PRINTER_IP, USERNAME, PASSWORD, MQTT_TOPIC")
	}

	if mqtt_debug {
		mqtt.DEBUG = log.New(os.Stdout, "[DEBUG] ", 0)
		mqtt.WARN = log.New(os.Stdout, "[WARN]  ", 0)
		mqtt.ERROR = log.New(os.Stdout, "[ERROR] ", 0)
	}

	log.Printf("Registering collector")
	bambulabs := newBambulabsCollector()
	prometheus.MustRegister(bambulabs)
	bambulabs.StartMQTTClient()

	log.Printf("Starting HTTP server on %s", HTTP_ADDR)

	http.HandleFunc("/", home)
	http.HandleFunc("/healthz", healthz)
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(HTTP_ADDR, nil))
}

const body = `<html>
				<head>
					<title>BambuLabs Exporter Metrics</title>
				</head>
				<body>
					<h1>BambuLabs Exporter</h1>
					<p><a href='` + "/metrics" + `'>metrics</a></p>
					<p><a href='` + "/healthz" + `'>healthz</a></p>
				</body>
			  </html>`

func home(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, body)
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}

func newTLSConfig() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true}
}
