# openpilot Integration

## Minimal
1. Add capnp definitions to cereal.
    * In openpilot cereal/custom.capnp replace CustomReserved17-19 with the Mapd
      types and structs in cereal/custom/custom.capnp line 65 to the end.
    * In openpilot cereal/log.capnp replace customReserved17-19 in the Event
      struct with the mapd types.
      ```
      mapdExtendedOut @143 :Custom.MapdExtendedOut;
      mapdIn @144 :Custom.MapdIn;
      mapdOut @145 :Custom.MapdOut;
      ```
2. Add mapdOut as a service in openpilot cereal/services.py and
   cereal/services.h
   ```python
   "mapdOut": (True, 20., 0.1),
   ```
   ```c++
   { "mapdOut", {"mapdOut", true, 20, 1}},
   ```
3. Add MapdSettings to openpilot common/params\_keys.h
   ```c++
    {"MapdSettings", {PERSISTENT, JSON}},
   ```
4. Add mapd binary to openpilot selfdrive directory
5. Add mapd to openpilot system/manager/process\_config.py
   ```python
   NativeProcess("mapd", "selfdrive", ["./mapd"], always_run),
   ```
5. In openpilot selfdrive/controls/plannerd.py add mapdOut to SubMaster
6. In openpilot selfdrive/controls/lib/longitudinal\_planner.py add the
   following code right before the force\_slow\_decel if statement:
   ```python
   if sm.valid['mapdOut']:
     if sm['mapdOut'].suggestedSpeed > 0 and v_cruise > sm['mapdOut'].suggestedSpeed:
       v_cruise = sm['mapdOut'].suggestedSpeed
   ```
7. ssh into the comma device with a running mapd instance and run
   `./selfdrive/mapd i` to download maps and configure mapd.
