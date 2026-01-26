#!/bin/bash
osmium tags-filter planet-daily.osm.pbf "nw/highway=motorway,trunk,primary,secondary,tertiary,unclassified,residential,motorway_link,trunk_link,primary_link,secondary_link,tertiary_link,traffic_signals,stop,crossing" -o filtered.osm.pbf --overwrite
