#!/bin/bash
set -e

find offline/*/* -type d -print0 | xargs -0 -P 16 -I{} tar -czvf {}.tar.gz {}

echo 'finished compressing files'
