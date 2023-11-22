#!/bin/bash

wget https://download.bbbike.org/osm/planet/planet-daily.osm.pbf

./generate_and_update_planet.sh
