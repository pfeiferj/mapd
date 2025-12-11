# Settings
Mapd settings are stored in the MapdSettings param. They can be directly
controlled through the param or through MapdIn cereal messages. See inputs.md
for more details about loading and saving settings. The following is details
about each setting, including location in the MapdSettings param and their
corresponding MapdIn actions.

### Speed Limit Control Enabled
When enabled mapd will use the speed limit to determine a suggested speed

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setSpeedLimitControl |
| MapdIn Field | bool |
| Param Key    | speed\_limit\_control\_enabled |

### Map Curve Speed Control Enabled
When enabled mapd will use map based curvature calculations to determine a suggested speed

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setMapCurveSpeedControl |
| MapdIn Field | bool |
| Param Key    | map\_curve\_speed\_control\_enabled |

### Vision Curve Speed Control Enabled
When enabled mapd will use vision model based curvature calculations to determine a suggested speed

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setVisionCurveSpeedControl |
| MapdIn Field | bool |
| Param Key    | vision\_curve\_speed\_control\_enabled |

### External Speed Limit Control Enabled
When enabled mapd will use fork provided speed limits to determine a suggested speed

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setExternalSpeedLimitControl |
| MapdIn Field | bool |
| Param Key    | external\_speed\_limit\_control\_enabled |

### Speed Limit Priority
Sets the prioritization method for available speed limits

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setSpeedLimitPriority |
| MapdIn Field | str |
| Values       | map, external, highest, lowest |
| Param Key    | speed\_limit\_priority |

### Speed Limit Offet
The offset that gets applied to a speed limit to determine a target speed

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setSpeedLimitOffset |
| MapdIn Field | float |
| Units        | meters/second |
| Param Key    | speed\_limit\_priority |

### Slow Down For Next Speed Limit
Determines if mapd will try to meet the upcoming speed limit before reaching it when the upcoming speed limit is lower than the current limit

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setSlowDownForNextSpeedLimit |
| MapdIn Field | bool |
| Param Key    | slow\_down\_for\_next\_speed\_limit |

### Speed Up For Next Speed Limit
Determines if mapd will try to meet the upcoming speed limit before reaching it when the upcoming speed limit is higher than the current limit

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setSpeedUpForNextSpeedLimit |
| MapdIn Field | bool |
| Param Key    | speed\_up\_for\_next\_speed\_limit |

### Speed Limit Change Requires Accept
Requires user acceptance of any speed limit changes before activating

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setSpeedLimitChangeRequiresAccept |
| MapdIn Field | bool |
| Param Key    | speed\_limit\_change\_requires\_accept |

### Press Gas To Accept Speed Limit
Pressing the gas will accept a speed limit change

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setPressGasToAcceptSpeedLimit |
| MapdIn Field | bool |
| Param Key    | press\_gas\_to\_accept\_speed\_limit |

### Press Gas To Override Speed Limit
Pressing the gas will override the speed limit to hold the current speed. Resets when the speed limit changes

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setPressGasToOverrideSpeedLimit |
| MapdIn Field | bool |
| Param Key    | press\_gas\_to\_override\_speed\_limit |

### Adjust Set Speed To Accept Speed Limit
Adjusting the set speed once in either direction will accept a speed limit change. Additional set speed changes reject the speed limit

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setAdjustSetSpeedToAcceptSpeedLimit |
| MapdIn Field | bool |
| Param Key    | adjust\_set\_speed\_to\_accept\_speed\_limit |

### Accept Speed Limit Timeout
The amount of time after a speed limit change is detected that accept inputs will be used. 0 is no limit

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setAcceptSpeedLimitTimeout |
| MapdIn Field | float |
| Units        | seconds |
| Param Key    | adjust\_set\_speed\_to\_accept\_speed\_limit |

### Vision Target Lateral Acceleration
The maximum lateral acceleration used in the Vision Curve Control speed calculations

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setVisionCurveTargetLatA |
| MapdIn Field | float |
| Units        | meters/second^2 |
| Param Key    | vision\_curve\_target\_lat\_a |

### Vision Minimum Target Velocity
The minimum speed that Vision Curve Control will request to drive

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setVisionCurveMinTargetV |
| MapdIn Field | float |
| Units        | meters/second |
| Param Key    | vision\_curve\_min\_target\_v |

### Mapd Enable Speed
The speed you can set your cruise control to that will then cause mapd features to engage

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setEnableSpeed |
| MapdIn Field | float |
| Units        | meters/second |
| Param Key    | enable\_speed |

### Use Enable Speed For Speed Limit
Determines whether the Mapd Enable Speed controls enabling of Speed Limit Control

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setSpeedLimitUseEnableSpeed |
| MapdIn Field | bool |
| Param Key    | speed\_limit\_use\_enable\_speed |

### Use Enable Speed for Map Curve Speed Control
Determines whether the Mapd Enable Speed controls enabling of Curve Speed Control

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setMapCurveUseEnableSpeed |
| MapdIn Field | bool |
| Param Key    | map\_curve\_use\_enable\_speed |

### Use Enable Speed for Vision Curve Speed Control
Determines whether the Mapd Enable Speed controls enabling of Vision Curve Speed Control

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setVisionCurveUseEnableSpeed |
| MapdIn Field | bool |
| Param Key    | vision\_curve\_use\_enable\_speed |

### Hold Speed Limit While Changing Set Speed
When enabled mapd will suggest using the speed limit while the cruise control speed is changing. This prevents speeding up while trying to reach the enable speed

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setHoldSpeedLimitWhileChangingSetSpeed |
| MapdIn Field | bool |
| Param Key    | hold\_speed\_limit\_while\_changing\_set\_speed |

### Hold Last Seen Speed Limit
When enabled mapd will use the last seen speed limit if it cannot determine a current speed limit

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setHoldLastSeenSpeedLimit |
| MapdIn Field | bool |
| Param Key    | hold\_last\_seen\_speed\_limit |

### Target Speed Jerk
The target amount of jerk to use when determining speed change activation distance (map curve and speed limit)

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setTargetSpeedJerk |
| MapdIn Field | float |
| Units        | meters/second^3 |
| Param Key    | target\_speed\_jerk |

### Target Speed Accel
The target amount of acceleration to use when determining speed change activation distance (map curve and speed limit)

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setTargetSpeedAccel |
| MapdIn Field | float |
| Units        | meters/second^2 |
| Param Key    | target\_speed\_accel |

### Target Speed Time Offset
An offset for the time before a target position to reach the target speed (map curve and speed limit)

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setTargetSpeedTimeOffset |
| MapdIn Field | float |
| Units        | seconds |
| Param Key    | target\_speed\_time\_offset |

### Map Curve Target Lateral Acceleration
An offset for the time before a target position to reach the target speed (map curve and speed limit)

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setMapCurveTargetLatA |
| MapdIn Field | float |
| Units        | meters/second^2 |
| Param Key    | map\_curve\_target\_lat\_a |

### Default Lane Width
The default lane width to use when determining if we are currently on a road

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setDefaultLaneWidth |
| MapdIn Field | float |
| Units        | meters |
| Param Key    | default\_lane\_width |

### Log Level
Modify how verbose logging will be for the mapd system

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setLogLevel |
| MapdIn Field | str |
| Values       | error, warn, info, debug |
| Param Key    | log\_level |

### Use JSON Logger
When true the logs will be output in a json format instead of a text format

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setLogJson |
| MapdIn Field | bool |
| Param Key    | log\_json |

### Log Source Location
When true the logs will include the file and line that wrote the log

| Item         | Description |
| ------------ | ----------- |
| MapdIn Type  | setLogSource |
| MapdIn Field | bool |
| Param Key    | log\_source |
