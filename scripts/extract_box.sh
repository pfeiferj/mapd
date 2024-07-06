#!/bin/bash
min_lon=$((${1}-1))
min_lat=$((${2}-1))
max_lon=$((${3}+1))
max_lat=$((${4}+1))
osmium extract --bbox ${min_lon},${min_lat},${max_lon},${max_lat} filtered.osm.pbf -o box.osm.pbf --overwrite
