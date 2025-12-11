# Mapd v2.x
This branch contains the docs and code for v2.x of mapd. v2.x is incompatible
with v1.x. v1.x can be found [here](https://github.com/pfeiferj/mapd/tree/v1.x).

Mapd is a component for use in forks of comma.ai's
[openpilot](https://github.com/commaai/openpilot). It uses openstreetmap data to
provide speed limit and curve speed adjustments to the openpilot fork.

## openpilot fork integration
openpilot fork integration is described in
[docs/integration.md](./docs/integration.md).

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
