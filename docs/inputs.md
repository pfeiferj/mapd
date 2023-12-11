# Inputs
## Memory Params
Most mapd inputs use params in the location /dev/shm/params to read and write
data between it and openpilot. In openpilot you can create an instance of
params that reads and writes to the correct location using the following:
```python
from openpilot.common.params import Params
mem_params = Params("/dev/shm/params")
```

### Position Data
To get location data into mapd you must write the gps coordinates to the
LastGPSPosition memory param. Note that it will read the normal /data/params
LastGPSPosition on initialization to pre-load the current area but after the
first read it will always read from /dev/shm/params. The expected format for the
gps position data is the following json:
```
{
    "latitude": -84.82069386553866,
    "longitude": 38.40504468653686,
    "bearing": 24
}
```
latitude, longitude, and bearing are all in degrees.

### Download Maps
Maps can be downloaded in one of two ways, by arbitrary bounding box or by
pre-defined locations.

#### Download by Bounding Box
To download an arbitrary bounding box write the bounding box to
/dev/shm/params/d/OSMDownloadBounds (OSMDownloadBounds memory param) using the
following json format:
```json
{
    "min_lon": -84.82069386553866,
    "min_lat": 38.40504468653686,
    "max_lon": -80.52071966199662,
    "max_lat": 41.97787393972939
}
```

When mapd downloads the specified bounding box it will clip the given bounding
box to regions of 2x2 degrees due to how the files are stored in the cloud. So
for the previous example the actual downloaded region would be:
```json
{
    "min_lon": -86,
    "min_lat": 38,
    "max_lon": -80,
    "max_lat": 42
}
```
#### Download by Pre-defined Locations
mapd contains pre-defined bounding boxes for ISO 3166-1 nations and US
states/territories. The nation bounding boxes are stored in
[nation_bounding_boxes.json](./nation_bounding_boxes.json) and the US
states/territories bounding boxes are stored in
[us_states_bounding_boxes.json](./us_states_bounding_boxes.json).

To download by nation you must write a json object containing a list of
abbreviations from the nation_bounding_boxes.json file to
/dev/shm/params/d/OSMDownloadLocations (OSMDownloadLocations memory param) using
the following format:
```json
{
    "nations": ["US", "CA"],
    "states": []
}
```
To download by US states/territories you must write a json object containing a
list of abbreviations from the states_bounding_boxes.json file to
/dev/shm/params/d/OSMDownloadLocations using the following format:
```json
{
    "nations": [],
    "states": ["OH", "KY", "WV"]
}
```

Note that when downloading the locations the bounding boxes specified in the
respective json files will actually be clipped to regions of 2x2 degrees the
same as when specifying an arbitrary bounding box to download. Also, when the
region completely surrounds another smaller region the smaller region will not
be excluded from the download.
