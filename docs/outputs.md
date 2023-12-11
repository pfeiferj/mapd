# Outputs

## Memory Params
Most mapd outputs use params in the location /dev/shm/params to read and write
data between it and openpilot. In openpilot you can create an instance of
params that reads and writes to the correct location using the following:
```python
from openpilot.common.params import Params
mem_params = Params("/dev/shm/params")
```

### Memory Param Definitions
* `RoadName`: utf-8 string. Based on the 'name' tag in osm, or if the name tag
is empty/does not exist it uses the 'ref' tag.
* `MapSpeedLimit`: the current speed limit in m/s as a utf-8 float string.
* `NextMapSpeedLimit`: output as json. GPS coordinates are in degrees,
speedlimit is in m/s. schema:
```
{
    "latitude": float,
    "longitude": float,
    "speedlimit": float
}
```
* `MapAdvisorySpeedLimit`: output as json. GPS coordinates are in degrees,
speedlimit is in m/s. schema:
```
{
    "start_latitude": float,
    "start_longitude": float,
    "end_latitude": float,
    "end_longitude": float,
    "speedlimit": float
}
```
* `NextMapAdvisorySpeedLimit`: output as json. GPS coordinates are in degrees,
speedlimit is in m/s. schema:
```
{
    "start_latitude": float,
    "start_longitude": float,
    "end_latitude": float,
    "end_longitude": float,
    "speedlimit": float
}
```
* `MapHazard`: output as json. The hazard name comes from the osm 'hazard'
tag. GPS coordinates are in degrees. schema:
```
{
    "start_latitude": float,
    "start_longitude": float,
    "end_latitude": float,
    "end_longitude": float,
    "hazard": float
}
```
* `NextMapHazard`: output as json. The hazard name comes from the osm 'hazard'
tag. GPS coordinates are in degrees. schema:
```
{
    "start_latitude": float,
    "start_longitude": float,
    "end_latitude": float,
    "end_longitude": float,
    "hazard": string
}
```
* `MapCurvatures`: output as json. Curvatures are output as k where `k = 1 /
radius (meters)`. A desired velocity can be found by using a target lateral
acceleration with the the formula `sqrt(target_lateral_acceleration/curvature)`.
GPS coordinates are in degrees. schema:
```
{
    "latitude": float,
    "longitude": float,
    "curvature": float
}
```
* `MapTargetVelocities`: output as json. Velocities are calculated using the
formula `sqrt(2/curvature)`. For a more customizable output use the
MapCurvatures instead. Volicity is in m/s and GPS coordinates are in degrees.
schema:
```
{
    "latitude": float,
    "longitude": float,
    "velocity": float
}
```

## Params
Some mapd outputs are regular params to make consuming in the UI easier.

### Param Definitions
* `OSMDownloadProgress`: output as json.
schema:
```
{
    "total_files": int,
    "downloaded_files": int,
    "locations_to_download": []string,
    "location_details": {
        "location_total_files": int,
        "location_downloaded_files": int
    }
}
```
