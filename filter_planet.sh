#!/bin/bash
osmium tags-filter planet-latest.osm.pbf "nw/highway!=pedestrian,path,cycleway,footway,track" -o filtered.osm.pbf --overwrite
