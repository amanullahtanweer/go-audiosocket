#!/bin/bash

# Convert raw audio files to WAV format
# Usage: ./convert_raw_to_wav.sh <raw_file> [sample_rate]

RAW_FILE="$1"
SAMPLE_RATE="${2:-8000}"  # Default to 8000 Hz

if [ -z "$RAW_FILE" ]; then
    echo "Usage: $0 <raw_file> [sample_rate]"
    exit 1
fi

OUTPUT_FILE="${RAW_FILE%.raw}.wav"

# Convert using sox or ffmpeg
if command -v sox &> /dev/null; then
    sox -r $SAMPLE_RATE -e signed -b 16 -c 1 "$RAW_FILE" "$OUTPUT_FILE"
elif command -v ffmpeg &> /dev/null; then
    ffmpeg -f s16le -ar $SAMPLE_RATE -ac 1 -i "$RAW_FILE" "$OUTPUT_FILE"
else
    echo "Neither sox nor ffmpeg found. Please install one of them."
    exit 1
fi

echo "Converted $RAW_FILE to $OUTPUT_FILE"