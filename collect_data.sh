#!/bin/bash
# Quick start script for raw data collection

cd "$(dirname "$0")"

# Activate virtual environment
source venv/bin/activate

# Run the collector
python3 raw_data_collector.py "$@"
