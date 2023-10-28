#!/bin/bash
for line in `find us-pacific/*/* -type f`;
do wrangler r2 object put osm-map-data/${line} -f=$line;
done
