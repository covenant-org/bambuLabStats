#!/usr/bin/env python3
"""
Bambu Lab Raw Data Collector

This script connects to your Bambu Lab printer via MQTT and stores
the raw JSON data locally for analysis.

File: raw_data_collector.py
Location: /Users/marlonespinosaperez/statsInfra/Bambulabs-Exporter/
"""

import json
import os
import ssl
import time
from datetime import datetime
from pathlib import Path

import paho.mqtt.client as mqtt
from dotenv import load_dotenv


class BambuLabDataCollector:
    """Collects and stores raw data from Bambu Lab printer."""

    # Stage ID mappings
    CURRENT_STAGE_IDS = {
        "default": "unknown",
        0: "printing",
        1: "auto_bed_leveling",
        2: "heatbed_preheating",
        3: "sweeping_xy_mech_mode",
        4: "changing_filament",
        5: "m400_pause",
        6: "paused_filament_runout",
        7: "heating_hotend",
        8: "calibrating_extrusion",
        9: "scanning_bed_surface",
        10: "inspecting_first_layer",
        11: "identifying_build_plate_type",
        12: "calibrating_micro_lidar",
        13: "homing_toolhead",
        14: "cleaning_nozzle_tip",
        15: "checking_extruder_temperature",
        16: "paused_user",
        17: "paused_front_cover_falling",
        18: "calibrating_micro_lidar",
        19: "calibrating_extrusion_flow",
        20: "paused_nozzle_temperature_malfunction",
        21: "paused_heat_bed_temperature_malfunction",
        22: "filament_unloading",
        23: "paused_skipped_step",
        24: "filament_loading",
        25: "calibrating_motor_noise",
        26: "paused_ams_lost",
        27: "paused_low_fan_speed_heat_break",
        28: "paused_chamber_temperature_control_error",
        29: "cooling_chamber",
        30: "paused_user_gcode",
        31: "motor_noise_showoff",
        32: "paused_nozzle_filament_covered_detected",
        33: "paused_cutter_error",
        34: "paused_first_layer_error",
        35: "paused_nozzle_clog",
        36: "check_absolute_accuracy_before_calibration",
        37: "absolute_accuracy_calibration",
        38: "check_absolute_accuracy_after_calibration",
        39: "calibrate_nozzle_offset",
        40: "bed_level_high_temperature",
        41: "check_quick_release",
        42: "check_door_and_cover",
        43: "laser_calibration",
        44: "check_plaform",
        45: "check_birdeye_camera_position",
        46: "calibrate_birdeye_camera",
        47: "bed_level_phase_1",
        48: "bed_level_phase_2",
        49: "heating_chamber",
        50: "heated_bedcooling",
        51: "print_calibration_lines",
        -1: "idle",
        255: "idle",
    }

    # Speed profile mappings
    SPEED_PROFILE = {
        1: "silent",
        2: "standard",
        3: "sport",
        4: "ludicrous"
    }

    def __init__(self, output_dir="raw_data"):
        """Initialize the collector.

        Args:
            output_dir: Directory to store raw data files
        """
        # Load environment variables
        load_dotenv()

        self.broker = os.getenv("BAMBU_PRINTER_IP")
        self.username = os.getenv("USERNAME")
        self.password = os.getenv("PASSWORD")
        self.mqtt_topic = os.getenv("MQTT_TOPIC")
        self.port = 8883

        # Validate configuration
        if not all([self.broker, self.username, self.password, self.mqtt_topic]):
            raise ValueError("Missing required environment variables in .env file")

        # Setup output directory
        self.output_dir = Path(output_dir)
        self.output_dir.mkdir(exist_ok=True)

        # Setup clean data directory
        self.clean_dir = Path(output_dir).parent / "clean_data"
        self.clean_dir.mkdir(exist_ok=True)

        # MQTT client
        self.client = None
        self.message_count = 0
        self.session_file = None
        self.clean_session_file = None

    def extract_clean_data(self, timestamp, message_number, topic, data):
        """Extract only the specified fields for clean data.

        Args:
            timestamp: ISO timestamp string
            message_number: Message sequence number
            topic: MQTT topic
            data: Full printer data dictionary

        Returns:
            Dictionary with only the specified clean fields
        """
        print_data = data.get('print', {})
        three_d_data = print_data.get('3D', {})

        # Get time values from mc_remaining_time and calculate total
        mc_remaining_time = print_data.get('mc_remaining_time', 0)
        mc_percent = print_data.get('mc_percent', 0)

        # Calculate total estimated time from remaining time and progress percentage
        total_time = None
        if mc_remaining_time and mc_percent and mc_percent > 0 and mc_percent < 100:
            total_time = round((mc_remaining_time * 100) / (100 - mc_percent))

        # Get status fields
        gcode_state = print_data.get('gcode_state')
        mc_print_sub_stage = print_data.get('mc_print_sub_stage')
        spd_lvl = print_data.get('spd_lvl')
        print_type = print_data.get('print_type')

        # Map sub_stage ID to description
        current_stage = self.CURRENT_STAGE_IDS.get(
            mc_print_sub_stage,
            self.CURRENT_STAGE_IDS.get("default")
        ) if mc_print_sub_stage is not None else None

        # Map speed level to profile name
        speed_profile = self.SPEED_PROFILE.get(spd_lvl) if spd_lvl is not None else None

        clean_data = {
            "timestamp": timestamp,
            "message_number": message_number,
            "topic": topic,
            "layer_num": three_d_data.get('layer_num'),
            "total_layer_num": three_d_data.get('total_layer_num'),
            "nozzle_temper": print_data.get('nozzle_temper'),
            "subtask_name": print_data.get('subtask_name'),
            "est_time": mc_remaining_time,  # Time remaining in minutes
            "total_time": total_time,  # Total estimated time in minutes
            "gcode_state": gcode_state,  # "RUNNING", "IDLE", etc.
            "current_stage": current_stage,  # Mapped from mc_print_sub_stage
            "speed_profile": speed_profile,  # "silent", "standard", "sport", "ludicrous"
            "print_type": print_type  # "cloud", "local", "idle", etc.
        }

        return clean_data

    def on_connect(self, client, userdata, flags, rc):
        """Callback when connected to MQTT broker."""
        if rc == 0:
            print(f"✓ Connected to {self.broker}:{self.port}")
            print(f"✓ Subscribing to topic: {self.mqtt_topic}")
            client.subscribe(self.mqtt_topic)
        else:
            print(f"✗ Connection failed with code {rc}")

    def on_message(self, client, userdata, msg):
        """Callback when a message is received."""
        try:
            # Parse the JSON payload
            payload = msg.payload.decode('utf-8')
            data = json.loads(payload)

            # Store individual message
            timestamp = datetime.now()
            self.message_count += 1

            # Create filename with timestamp
            filename = f"message_{timestamp.strftime('%Y%m%d_%H%M%S_%f')}.json"
            filepath = self.output_dir / filename

            # Add metadata
            data_with_metadata = {
                "timestamp": timestamp.isoformat(),
                "message_number": self.message_count,
                "topic": msg.topic,
                "data": data
            }

            # Write to individual file (raw data)
            with open(filepath, 'w') as f:
                json.dump(data_with_metadata, f, indent=2)

            # Also append to session file
            if self.session_file:
                self.session_file.write(json.dumps(data_with_metadata) + '\n')
                self.session_file.flush()

            # Extract and save clean data
            clean_data = self.extract_clean_data(
                timestamp.isoformat(),
                self.message_count,
                msg.topic,
                data
            )

            # Write clean data to individual file
            clean_filepath = self.clean_dir / filename
            with open(clean_filepath, 'w') as f:
                json.dump(clean_data, f, indent=2)

            # Also append to clean session file
            if self.clean_session_file:
                self.clean_session_file.write(json.dumps(clean_data) + '\n')
                self.clean_session_file.flush()

            # Print summary
            command = data.get('print', {}).get('command', 'unknown')
            stage = data.get('print', {}).get('mc_print_stage', 'N/A')
            percent = data.get('print', {}).get('mc_percent', 'N/A')

            print(f"[{self.message_count}] {timestamp.strftime('%H:%M:%S')} | "
                  f"Command: {command} | Stage: {stage} | Progress: {percent}% | "
                  f"Layer: {clean_data['layer_num']}/{clean_data['total_layer_num']}")

        except json.JSONDecodeError as e:
            print(f"✗ Failed to decode JSON: {e}")
            # Save raw payload for debugging
            error_file = self.output_dir / f"error_{datetime.now().strftime('%Y%m%d_%H%M%S')}.txt"
            with open(error_file, 'w') as f:
                f.write(payload)
        except Exception as e:
            print(f"✗ Error processing message: {e}")

    def on_disconnect(self, client, userdata, rc):
        """Callback when disconnected from MQTT broker."""
        print(f"\n✗ Disconnected from broker (code {rc})")

    def collect(self, duration=None, max_messages=None):
        """Start collecting data.

        Args:
            duration: Optional duration in seconds (None = run forever)
            max_messages: Optional max number of messages to collect
        """
        # Setup MQTT client
        self.client = mqtt.Client()
        self.client.username_pw_set(self.username, self.password)
        self.client.on_connect = self.on_connect
        self.client.on_message = self.on_message
        self.client.on_disconnect = self.on_disconnect

        # Setup TLS
        self.client.tls_set(cert_reqs=ssl.CERT_NONE)
        self.client.tls_insecure_set(True)

        # Create session files
        session_timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
        session_filepath = self.output_dir / f"session_{session_timestamp}.jsonl"
        clean_session_filepath = self.clean_dir / f"session_{session_timestamp}.jsonl"

        self.session_file = open(session_filepath, 'w')
        self.clean_session_file = open(clean_session_filepath, 'w')

        print(f"\n{'='*60}")
        print(f"Bambu Lab Raw Data Collector")
        print(f"{'='*60}")
        print(f"Broker:       {self.broker}:{self.port}")
        print(f"Topic:        {self.mqtt_topic}")
        print(f"Raw Output:   {self.output_dir.absolute()}")
        print(f"Clean Output: {self.clean_dir.absolute()}")
        print(f"Session:      {session_filepath.name}")
        print(f"{'='*60}\n")

        try:
            # Connect to broker
            print(f"Connecting to {self.broker}...")
            self.client.connect(self.broker, self.port, 60)

            # Start loop
            self.client.loop_start()

            # Run for specified duration or until max messages
            start_time = time.time()
            while True:
                if duration and (time.time() - start_time) >= duration:
                    print(f"\n✓ Duration limit reached ({duration}s)")
                    break
                if max_messages and self.message_count >= max_messages:
                    print(f"\n✓ Message limit reached ({max_messages})")
                    break
                time.sleep(0.1)

        except KeyboardInterrupt:
            print("\n\n✓ Interrupted by user")
        except Exception as e:
            print(f"\n✗ Error: {e}")
        finally:
            # Cleanup
            self.client.loop_stop()
            self.client.disconnect()
            if self.session_file:
                self.session_file.close()
            if self.clean_session_file:
                self.clean_session_file.close()

            print(f"\n{'='*60}")
            print(f"Collection Summary")
            print(f"{'='*60}")
            print(f"Messages collected:    {self.message_count}")
            print(f"Raw session file:      {session_filepath}")
            print(f"Raw individual files:  {self.output_dir.absolute()}")
            print(f"Clean session file:    {clean_session_filepath}")
            print(f"Clean individual files:{self.clean_dir.absolute()}")
            print(f"{'='*60}\n")


def main():
    """Main entry point."""
    import argparse

    parser = argparse.ArgumentParser(
        description="Collect raw data from Bambu Lab printer via MQTT"
    )
    parser.add_argument(
        "-d", "--duration",
        type=int,
        help="Collection duration in seconds (default: run forever)",
        default=None
    )
    parser.add_argument(
        "-n", "--max-messages",
        type=int,
        help="Maximum number of messages to collect",
        default=None
    )
    parser.add_argument(
        "-o", "--output",
        type=str,
        help="Output directory for raw data (default: raw_data)",
        default="raw_data"
    )

    args = parser.parse_args()

    # Create collector and run
    collector = BambuLabDataCollector(output_dir=args.output)
    collector.collect(duration=args.duration, max_messages=args.max_messages)


if __name__ == "__main__":
    main()
