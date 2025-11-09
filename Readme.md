# Mapd v2.x
This branch contains the docs and code for v2.x of mapd. v2.x is incompatible
with v1.x. v1.x can be found [here](https://github.com/pfeiferj/mapd/tree/v1.x).

Mapd is a component for use in forks of comma.ai's
[openpilot](https://github.com/commaai/openpilot). It uses openstreetmap data to
provide speed limit and curve speed adjustments to the openpilot fork.

## Differences from mapd v1.x
v2.x is a major rewrite of mapd to better integrate with openpilot. Due to mapd
being written in go, it was not able to use the interprocess communication
library that comma.ai made for openpilot. In v1.x this was overcome by
implementing the much simpler Params interface that openpilot uses to store
persistent data. This interface was pointed at /dev/shm in order to use memory
instead of the disk to increase speed. This however is still not ideal as the
Params interface is a blocking interface and the inputs and outputs from mapd
were close to the limit of what would work without causing openpilot issues.

In v2.x mapd now uses the same ipc interface as openpilot. This was accomplished
by rewriting comma.ai's [msgq](https://github.com/commaai/msgq) library in go.
The go implementation can be found [here](https://github.com/pfeiferj/gomsgq).
This gives v2.x several advantages in how it operates compared to v1.x:

* msgq is non-blocking. This means that mapd no longer causes unnecessary
  latency in the controls loop of openpilot.

* Mapd can now directly read openpilot's state. This means it no longer needs
  code added to openpilot to read and resend data to mapd.

* The mapd outputs can be stored in openpilot's log system. This means that the
  outputs can be replayed using the standard openpilot replay system giving
  several benefits:
    * clip generators that use the openpilot ui can now show the mapd based ui
    elements.
    * When debugging, the exact state of both openpilot and mapd can be seen in
    sync.
    * When debugging, a mapd instance can use the openpilot replay data as
    inputs. This enables checking if fixes for issues work by comparing how the
    new mapd instance behaves compared to the actual output of mapd on a device.

* Mapd now has more headroom in its ipc. This means that logic that previously
  was implemented on the python side of the openpilot fork can now be directly
  integrated into mapd itself. This should simplify the openpilot fork codebase
  and make it easier to maintain.

* Due to directly reading the openpilot state, mapd can now implement features
  that use additional data from openpilot without worrying about breaking
  compatiblity with forks.

* Mapd's update loop can now run faster and be more stateful as it now can
  actually know if the input data is new.


There is, however, a con to this approach. The openpilot fork _must_ add the mapd
capnp definitions to their cereal capnp definitions. mapd uses multiple of the
"CustomReserved" messages in cereal and the fork must ensure that they replace
the same CustomReserved messages to be able to communicate with mapd. In the
event of a conflict with the fork, the forks only options are to either adjust
their code to remove the conflict, or maintain their own fork of mapd that
updates mapd's definitions to not conflict.



## Disclaimers and Acknowledgements
* "openpilot" and "msgq" are trademarks of comma.ai. mapd is in no way affiliated with
  comma.ai, openpilot, or msgq.

* The capnp definitions in the cereal package are modifications of the
  definitions in the cereal module in
  [openpilot](https://github.com/commaai/openpilot/tree/master/cereal). openpilot
  is released under the MIT license.

* Many of the ideas in this code were built by referencing the
  [move-fast implementation](https://github.com/move-fast/openpilot/tree/b170d1bc123a0cf2b872050fbd5e2eecd1b23e22/selfdrive/mapd/lib)
  of an openpilot map daemon. The move-fast implementation was released under the
  MIT license.

* Other ideas in this implementation came from the wider openpilot fork
  community. Some notable mentions are:
    * [sunnypilot](https://github.com/sunnypilot/sunnypilot)
    * [frogpilot](https://github.com/FrogAi/FrogPilot)
    * [dragonpilot](https://github.com/dragonpilot/dragonpilot)
