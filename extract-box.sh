#!/bin/bash
osmium extract --bbox ${1},${2},${3},${4} filtered.osm.pbf -o box.osm.pbf --overwrite
