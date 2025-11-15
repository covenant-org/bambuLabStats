# Bambu Lab Raw Data Collector

## Overview

This tool queries raw MQTT data directly from your Bambu Lab printer and stores it locally for analysis.

## Files

- **`main.go`** (lines 330-356): The Go exporter's MQTT message handler - this is what the Docker container uses
- **`raw_data_collector.py`**: Python script to collect and store raw printer data
- **`collect_data.sh`**: Quick start script (uses virtual environment)

## How It Works

### The Exporter (main.go)
The existing Docker exporter:
1. Connects to printer via MQTT on port 8883 (TLS)
2. Subscribes to topic: `device/YOUR_SERIAL/report`
3. Receives JSON messages every ~1 second
4. Parses and exposes as Prometheus metrics on port 9101

### The Raw Data Collector (Python)
The new collector does the same thing but stores raw JSON:
1. Connects to the same MQTT broker
2. Subscribes to the same topic
3. Stores each message as individual JSON file
4. Also maintains a session log (JSONL format)

## Usage

### Quick Start

```bash
# Collect 10 messages
./collect_data.sh -n 10

# Collect for 60 seconds
./collect_data.sh -d 60

# Run continuously (Ctrl+C to stop)
./collect_data.sh

# Custom output directory
./collect_data.sh -o my_data -n 5
```

### Direct Python Usage

```bash
# Activate virtual environment
source venv/bin/activate

# Run collector
python3 raw_data_collector.py -n 10
```

### Options

- `-n, --max-messages NUM` - Collect NUM messages then stop
- `-d, --duration SECONDS` - Run for SECONDS then stop
- `-o, --output DIR` - Output directory (default: `raw_data`)
- `-h, --help` - Show help

## Output Structure

### Individual Message Files
```
raw_data/
├── message_20251114_194918_614942.json  # Individual messages
├── message_20251114_194919_643986.json
├── message_20251114_194920_581777.json
└── session_20251114_194917.jsonl        # All messages in one file
```

### Message Format
```json
{
  "timestamp": "2025-11-14T19:49:18.614942",
  "message_number": 1,
  "topic": "device/0938AC580201618/report",
  "data": {
    "print": {
      "command": "push_status",
      "mc_print_stage": "2",      // Stage: 0=idle, 2=printing
      "mc_percent": 0,             // Print progress %
      "mc_remaining_time": 0,      // Minutes remaining
      "nozzle_temper": 270,        // Current nozzle temp
      "bed_temper": 65,            // Current bed temp
      "layer_num": 0,              // Current layer
      "total_layer_num": 450,      // Total layers
      "wifi_signal": "-47dBm",     // WiFi signal strength
      // ... hundreds more fields
    }
  }
}
```

## Raw Data Fields

The printer sends ~200+ fields in each message. Key fields:

### Print Status
- `print.command` - Command type (usually "push_status")
- `print.gcode_state` - Text state: "idle", "running", "pause", "finish", "failed"
- `print.mc_print_stage` - Numeric stage (0=idle, 2=printing, etc.)
- `print.mc_print_sub_stage` - Sub-stage (0=printing, 1=leveling, etc.)
- `print.mc_percent` - Progress percentage
- `print.mc_remaining_time` - Minutes remaining

### Temperatures
- `print.nozzle_temper` - Current nozzle temperature
- `print.nozzle_target_temper` - Target nozzle temperature
- `print.bed_temper` - Current bed temperature
- `print.bed_target_temper` - Target bed temperature
- `print.chamber_temper` - Chamber temperature

### Print Info
- `print.layer_num` - Current layer
- `print.3D.total_layer_num` - Total layers
- `print.subtask_name` - Name of current print job
- `print.wifi_signal` - WiFi signal strength

### AMS (Automatic Material System)
- `print.ams.ams[].humidity` - Humidity per AMS unit
- `print.ams.ams[].temp` - Temperature per AMS unit
- `print.ams.ams[].tray[].tray_color` - Filament colors
- `print.ams.ams[].tray[].tray_type` - Filament types

### Fans
- `print.big_fan1_speed`, `print.big_fan2_speed` - Chamber fans
- `print.cooling_fan_speed` - Part cooling fan
- `print.heatbreak_fan_speed` - Hotend fan

## Analysis Examples

### Count Messages per Session
```bash
cat raw_data/session_*.jsonl | wc -l
```

### Extract Print Stages
```bash
cat raw_data/session_*.jsonl | jq '.data.print.mc_print_stage' | sort | uniq -c
```

### Get Temperature Stats
```bash
cat raw_data/session_*.jsonl | jq '.data.print.nozzle_temper' | \
  awk '{sum+=$1; count++} END {print "Avg:", sum/count, "°C"}'
```

### Find Print Progress
```bash
cat raw_data/session_*.jsonl | jq -r '[.timestamp, .data.print.mc_percent] | @tsv'
```

## Comparison: Exporter vs Collector

| Feature | Go Exporter (Docker) | Python Collector |
|---------|---------------------|------------------|
| **Purpose** | Metrics for Prometheus | Raw data analysis |
| **Format** | Prometheus text format | JSON files |
| **Storage** | Prometheus TSDB | Local JSON files |
| **Data** | ~20 metrics | 200+ fields |
| **Retention** | 365 days (compressed) | Unlimited (uncompressed) |
| **Query** | PromQL | jq, Python, manual |
| **Use Case** | Monitoring, alerting | Deep analysis, debugging |

## Requirements

- Python 3.7+
- paho-mqtt library
- python-dotenv library
- Access to `.env` file with printer credentials

## Environment Variables

Uses the same `.env` file as the Docker exporter:

```env
BAMBU_PRINTER_IP=192.168.50.213
USERNAME=bblp
PASSWORD=your_access_code
MQTT_TOPIC=device/YOUR_SERIAL/report
```

## Troubleshooting

### Connection Failed
- Check printer IP is correct
- Verify access code in `.env`
- Ensure printer is powered on and connected to network

### No Messages Received
- Verify MQTT topic matches your printer serial
- Check if printer is sending data (might be in sleep mode)
- Ensure no firewall blocking port 8883

### Permission Denied on collect_data.sh
```bash
chmod +x collect_data.sh
```

## Use Cases

1. **Debugging**: Capture raw data when issues occur
2. **Analysis**: Deep dive into printer behavior
3. **Feature Discovery**: Find undocumented fields
4. **Testing**: Validate exporter logic against raw data
5. **Historical**: Keep complete records of print sessions

## Safety Notes

- Raw data files can be large (14KB per message)
- 1 message/second = ~5MB/hour, ~120MB/day
- Clean up old files regularly
- Consider compressing session files

## Example Session

```bash
$ ./collect_data.sh -n 20

============================================================
Bambu Lab Raw Data Collector
============================================================
Broker:  192.168.50.213:8883
Topic:   device/0938AC580201618/report
Output:  /Users/marlonespinosaperez/statsInfra/Bambulabs-Exporter/raw_data
Session: session_20251114_194917.jsonl
============================================================

Connecting to 192.168.50.213...
✓ Connected to 192.168.50.213:8883
✓ Subscribing to topic: device/0938AC580201618/report
[1] 19:49:18 | Command: push_status | Stage: 2 | Progress: 77%
[2] 19:49:19 | Command: push_status | Stage: 2 | Progress: 77%
[3] 19:49:20 | Command: push_status | Stage: 2 | Progress: 78%
...
[20] 19:49:37 | Command: push_status | Stage: 2 | Progress: 78%

✓ Message limit reached (20)

============================================================
Collection Summary
============================================================
Messages collected: 20
Session file:       raw_data/session_20251114_194917.jsonl
Individual files:   /Users/marlonespinosaperez/statsInfra/Bambulabs-Exporter/raw_data
============================================================
```
