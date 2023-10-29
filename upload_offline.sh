#!/bin/bash
for line in `find offline/*/*.tar.gz -type f`;
do wrangler r2 object put osm-map-data/${line} -f=$line;
done
