# Bambu Lab Printer Status Guide

## Your Current Printer Status

**File Location**: `/Users/marlonespinosaperez/statsInfra/Bambulabs-Exporter/`

### Current Live Metrics (from your printer right now):
```
mc_percent_metric: 77%
mc_print_stage_metric: 2
mc_print_sub_stage_metric: 0
nozzle_temper_metric: 270°C
layer_number_metric: 288
mc_remaining_time_metric: 30 minutes
```

## What the Printer Returns

### Two Different Status Systems:

#### 1. **gcode_state** (String field - Main Status)
These are the possible string values your printer sends:
- `"idle"` - Printer is idle, not printing
- `"running"` - **Actively printing** ← THIS IS YOUR CURRENT STATE
- `"pause"` - Print is paused
- `"finish"` - Print completed
- `"failed"` - Print failed
- `"prepare"` - Preparing to print
- `"init"` - Initializing
- `"slicing"` - Slicing file
- `"offline"` - Printer offline
- `"unknown"` - Unknown state

#### 2. **mc_print_stage** (Numeric field - currently exposed as metric)
The exporter converts this string to a number. Based on your data:
- **Stage 2** with 77% progress and 270°C = **PRINTING/RUNNING**

## The Problem

The dashboard value mapping was **INCORRECT**:
```
Wrong mapping (what we had):
  2 → "PAUSED" ❌

Correct mapping (what it should be):
  2 → "PRINTING" ✓
```

## Current Stage Substates (from Home Assistant integration)

When your printer is actively working, it can be in different substages:
- 0: printing
- 1: auto_bed_leveling
- 2: heatbed_preheating
- 3: sweeping_xy_mech_mode
- 4: changing_filament
- 5: m400_pause
- 6: paused_filament_runout
- 7: heating_hotend
- 8: calibrating_extrusion
- 9: scanning_bed_surface
- 10: inspecting_first_layer
- 11: identifying_build_plate_type
- 12: calibrating_micro_lidar
- 13: homing_toolhead
- 14: cleaning_nozzle_tip
- 15: checking_extruder_temperature
- 16: paused_user
- 17: paused_front_cover_falling
- 19: calibrating_extrusion_flow
- 20: paused_nozzle_temperature_malfunction
- 21: paused_heat_bed_temperature_malfunction
- 22: filament_unloading
- 23: paused_skipped_step
- 24: filament_loading
- 25: calibrating_motor_noise
- 26: paused_ams_lost
- 27: paused_low_fan_speed_heat_break
- 28: paused_chamber_temperature_control_error
- 29: cooling_chamber

**Your current substage: 0** = actively printing

## Recommended Dashboard Mapping

Based on evidence from your printer and the Home Assistant integration:

### Simple Status (using mc_print_stage):
The exporter currently converts numeric strings. Based on typical patterns:
- **Value 0 or very low %**: IDLE
- **Value 2 with progress**: PRINTING/RUNNING
- **Values 3-5**: Various prep/pause states

### Better Approach:
Use **gcode_state** (string field) which gives you exact text like:
- "idle", "running", "pause", "finish", "failed"

However, the current exporter converts mc_print_stage to a number, losing the text information.

## Files Created:
1. **current_printer_status.txt** - Your live printer metrics
2. **bambu_status_codes.py** - Home Assistant integration status constants
3. **PRINTER_STATUS_GUIDE.md** - This guide

## Next Steps

**Fix the dashboard mapping** to show:
- Stage 2 (with progress > 0) = "PRINTING"
- Can also check mc_percent_metric to distinguish IDLE from PRINTING
