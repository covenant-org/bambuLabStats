package main

/* BambuLabs Slim Exporter - Clean Data Only
 *
 * Modified to export only the 13 clean data fields
 * Original by Aetrius Tyler B and Matt Beckett
 * Modified by Scott Baker (https://www.smbaker.com/)
 * Slimmed down for covenant-org/bambuLabStats
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

// Stage ID mappings
var CURRENT_STAGE_IDS = map[int]string{
	0:   "printing",
	1:   "auto_bed_leveling",
	2:   "heatbed_preheating",
	3:   "sweeping_xy_mech_mode",
	4:   "changing_filament",
	5:   "m400_pause",
	6:   "paused_filament_runout",
	7:   "heating_hotend",
	8:   "calibrating_extrusion",
	9:   "scanning_bed_surface",
	10:  "inspecting_first_layer",
	11:  "identifying_build_plate_type",
	12:  "calibrating_micro_lidar",
	13:  "homing_toolhead",
	14:  "cleaning_nozzle_tip",
	15:  "checking_extruder_temperature",
	16:  "paused_user",
	17:  "paused_front_cover_falling",
	18:  "calibrating_micro_lidar",
	19:  "calibrating_extrusion_flow",
	20:  "paused_nozzle_temperature_malfunction",
	21:  "paused_heat_bed_temperature_malfunction",
	22:  "filament_unloading",
	23:  "paused_skipped_step",
	24:  "filament_loading",
	25:  "calibrating_motor_noise",
	26:  "paused_ams_lost",
	27:  "paused_low_fan_speed_heat_break",
	28:  "paused_chamber_temperature_control_error",
	29:  "cooling_chamber",
	30:  "paused_user_gcode",
	31:  "motor_noise_showoff",
	32:  "paused_nozzle_filament_covered_detected",
	33:  "paused_cutter_error",
	34:  "paused_first_layer_error",
	35:  "paused_nozzle_clog",
	36:  "check_absolute_accuracy_before_calibration",
	37:  "absolute_accuracy_calibration",
	38:  "check_absolute_accuracy_after_calibration",
	39:  "calibrate_nozzle_offset",
	40:  "bed_level_high_temperature",
	41:  "check_quick_release",
	42:  "check_door_and_cover",
	43:  "laser_calibration",
	44:  "check_plaform",
	45:  "check_birdeye_camera_position",
	46:  "calibrate_birdeye_camera",
	47:  "bed_level_phase_1",
	48:  "bed_level_phase_2",
	49:  "heating_chamber",
	50:  "heated_bedcooling",
	51:  "print_calibration_lines",
	-1:  "idle",
	255: "idle",
}

// Speed profile mappings
var SPEED_PROFILE = map[int]string{
	1: "silent",
	2: "standard",
	3: "sport",
	4: "ludicrous",
}

type bambulabsCollector struct {
	layerNumMetric        *prometheus.Desc
	totalLayerNumMetric   *prometheus.Desc
	nozzleTemperMetric    *prometheus.Desc
	subtaskNameMetric     *prometheus.Desc
	estTimeMetric         *prometheus.Desc
	totalTimeMetric       *prometheus.Desc
	gcodeStateMetric      *prometheus.Desc
	currentStageMetric    *prometheus.Desc
	speedProfileMetric    *prometheus.Desc
	printTypeMetric       *prometheus.Desc
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
	if err != nil {
		return false
	}
	return b
}

// Initializes every descriptor and returns a pointer to the collector
func newBambulabsCollector() *bambulabsCollector {
	return &bambulabsCollector{
		layerNumMetric: prometheus.NewDesc("layer_num",
			"Current layer number",
			nil, nil,
		),
		totalLayerNumMetric: prometheus.NewDesc("total_layer_num",
			"Total number of layers",
			nil, nil,
		),
		nozzleTemperMetric: prometheus.NewDesc("nozzle_temper",
			"Nozzle temperature in Celsius",
			nil, nil,
		),
		subtaskNameMetric: prometheus.NewDesc("subtask_name",
			"Name of the current print job",
			[]string{"name"}, nil,
		),
		estTimeMetric: prometheus.NewDesc("est_time",
			"Estimated time remaining in minutes",
			nil, nil,
		),
		totalTimeMetric: prometheus.NewDesc("total_time",
			"Total estimated print time in minutes",
			nil, nil,
		),
		gcodeStateMetric: prometheus.NewDesc("gcode_state",
			"GCode state (RUNNING, IDLE, etc.)",
			[]string{"state"}, nil,
		),
		currentStageMetric: prometheus.NewDesc("current_stage",
			"Current print stage description",
			[]string{"stage"}, nil,
		),
		speedProfileMetric: prometheus.NewDesc("speed_profile",
			"Speed profile (silent, standard, sport, ludicrous)",
			[]string{"profile"}, nil,
		),
		printTypeMetric: prometheus.NewDesc("print_type",
			"Print type (cloud, local, idle, etc.)",
			[]string{"type"}, nil,
		),
	}
}

func (collector *bambulabsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.layerNumMetric
	ch <- collector.totalLayerNumMetric
	ch <- collector.nozzleTemperMetric
	ch <- collector.subtaskNameMetric
	ch <- collector.estTimeMetric
	ch <- collector.totalTimeMetric
	ch <- collector.gcodeStateMetric
	ch <- collector.currentStageMetric
	ch <- collector.speedProfileMetric
	ch <- collector.printTypeMetric
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

	log.Printf("Subscribed to topic %s", mqtt_topic)
}

// Collect implements the collect function for clean data only
func (collector *bambulabsCollector) Collect(ch chan<- prometheus.Metric) {
	if !connected {
		// If we were disconnected, then reconnect the MQTT client
		collector.StartMQTTClient()
		return
	}

	// Extract clean data fields
	layerNum := float64(data.Print.ThreeD.LayerNum)
	totalLayerNum := float64(data.Print.ThreeD.TotalLayerNum)
	nozzleTemper := data.Print.NozzleTemper
	subtaskName := data.Print.SubtaskName
	mcRemainingTime := float64(data.Print.McRemainingTime)
	mcPercent := data.Print.McPercent
	gcodeState := data.Print.GcodeState
	mcPrintSubStage := data.Print.McPrintSubStage
	spdLvl := data.Print.SpdLvl
	printType := data.Print.PrintType

	// Calculate total time from remaining time and percentage
	// Printer sends total_time: 0, so we must calculate it
	var totalTime float64
	if mcRemainingTime > 0 && mcPercent > 0 && mcPercent < 100 {
		totalTime = (mcRemainingTime * 100) / float64(100-mcPercent)
	}

	// Map current stage ID to description
	currentStage, ok := CURRENT_STAGE_IDS[mcPrintSubStage]
	if !ok {
		currentStage = "unknown"
	}

	// Map speed level to profile name
	speedProfile, ok := SPEED_PROFILE[spdLvl]
	if !ok {
		speedProfile = "unknown"
	}

	// Create metrics
	ch <- prometheus.MustNewConstMetric(collector.layerNumMetric, prometheus.GaugeValue, layerNum)
	ch <- prometheus.MustNewConstMetric(collector.totalLayerNumMetric, prometheus.GaugeValue, totalLayerNum)
	ch <- prometheus.MustNewConstMetric(collector.nozzleTemperMetric, prometheus.GaugeValue, nozzleTemper)
	ch <- prometheus.MustNewConstMetric(collector.subtaskNameMetric, prometheus.GaugeValue, 1, subtaskName)
	ch <- prometheus.MustNewConstMetric(collector.estTimeMetric, prometheus.GaugeValue, mcRemainingTime)
	ch <- prometheus.MustNewConstMetric(collector.totalTimeMetric, prometheus.GaugeValue, totalTime)
	ch <- prometheus.MustNewConstMetric(collector.gcodeStateMetric, prometheus.GaugeValue, 1, gcodeState)
	ch <- prometheus.MustNewConstMetric(collector.currentStageMetric, prometheus.GaugeValue, 1, currentStage)
	ch <- prometheus.MustNewConstMetric(collector.speedProfileMetric, prometheus.GaugeValue, 1, speedProfile)
	ch <- prometheus.MustNewConstMetric(collector.printTypeMetric, prometheus.GaugeValue, 1, printType)
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
	log.Printf("Starting Slim Exporter (Clean Data Only): %s", time.Now().String())

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

	log.Printf("Registering slim collector (13 clean data metrics only)")
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
					<title>BambuLabs Slim Exporter - Clean Data Only</title>
				</head>
				<body>
					<h1>BambuLabs Slim Exporter</h1>
					<p>Exporting 13 clean data metrics only</p>
					<p><a href='` + "/metrics" + `'>metrics</a></p>
					<p><a href='` + "/healthz" + `'>healthz</a></p>
					<hr>
					<p>Clean Data Metrics:</p>
					<ul>
						<li>layer_num - Current layer number</li>
						<li>total_layer_num - Total layers</li>
						<li>nozzle_temper - Nozzle temperature</li>
						<li>subtask_name - Print job name</li>
						<li>est_time - Time remaining (minutes)</li>
						<li>total_time - Total time (minutes)</li>
						<li>gcode_state - Print state</li>
						<li>current_stage - Current stage</li>
						<li>speed_profile - Speed setting</li>
						<li>print_type - Print type</li>
					</ul>
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
