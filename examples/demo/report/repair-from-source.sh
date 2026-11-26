#!/usr/bin/env bash
set -euo pipefail

ffmpeg -y -hide_banner -loglevel error -i /home/hj/userfeedback/uid4-0026/examples/demo/fixtures/source-prores4444-alpha.mov -map 0:v:0 -an -c:v prores_ks -profile:v 4444 -pix_fmt yuva444p10le -alpha_bits 16 -color_primaries bt709 -color_trc bt709 -colorspace bt709 /home/hj/userfeedback/uid4-0026/examples/demo/report/repaired-alpha-proxy.mov
