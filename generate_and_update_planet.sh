#!/bin/bash
MIN_LON=-180
MIN_LAT=-90
MAX_LON=180
MAX_LAT=90

for (( i=$MIN_LON; i <= $MAX_LON; i+=20 )) 
do
  for (( j=$MIN_LAT; j <= $MAX_LAT; j += 20 )) 
  do
    max_lon=$(($i+20))
    max_lat=$(($j+20))
    ./extract_box.sh $i $j $max_lon $max_lat
    ./add_locations.sh
    go run . --generate
    ./compress_offline.sh
    ./upload_offline.sh
    rm -r offline
  done

done
