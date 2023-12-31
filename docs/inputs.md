# Inputs
## Memory Params
Most mapd inputs use params in the location /dev/shm/params to read and write
data between it and openpilot. In openpilot you can create an instance of
params that reads and writes to the correct location using the following:
```python
from openpilot.common.params import Params
mem_params = Params("/dev/shm/params")
```

### Log Level
The log level for mapd can be set by writing to the MapdLogLevel param. mapd
reads the normal persistent MapdLogLevel param once at start and checks the
MapdLogLevel memory param each loop to allow for changing the level while the
process is running. The default level is `info`.

The level should be written as one of the following strings:
* `debug`
* `info`
* `warn`
* `error`
* `disabled`

### Pretty Logs
By default mapd outputs structured json logs. To print in a more human friendly
format you can set the MapdPrettyLog param to 1 (in openpilot this would be
equivalent to put\_bool("MapdPrettyLog", true)). mapd reads the persistent param
once at start and checks the mem param location each loop to allow for changing
the value while running. Setting the mem param to 0 will revert back to
structured logging.

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

### Target Lateral Accel for Curvatures
The default lateral accel used when calculating velocities for map based turn
speed control is 2.0 m/s^2. This value can be configured using the `MapTargetLatA`
param and memory param. The regular persistent param is only read once when the
process starts, the memory param is read every loop to allow updating the value
while the process is running.

* A change of +- 0.1 m/s^2 results in the velocity being raised or lowered by
about 1 mph. 
* In general I suggest picking a value a few tenths below what the max reported
  max accel for your car is in [torque_data/params.yaml](https://github.com/commaai/openpilot/blob/master/selfdrive/car/torque_data/params.yaml).
  I have also created a copy of this data in a slightly easier to read format
  [here](./torque_data.md).
