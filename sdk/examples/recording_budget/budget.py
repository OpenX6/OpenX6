# SPDX-License-Identifier: Apache-2.0
"""Compute storage use for the selected total video rate (SI Mbps, binary GiB)."""
import json
import sys

rates = {'standard': 180, 'high': 220, 'max': 260}
rate = rates[sys.argv[1]]
bytes_per_hour = rate * 1_000_000 * 3600 // 8
print(json.dumps({
    'mbps': rate,
    'video_gib_per_hour': round(bytes_per_hour / 2**30, 2),
    'minutes_per_128_gib': round(128 * 2**30 * 60 / bytes_per_hour, 1),
}))
