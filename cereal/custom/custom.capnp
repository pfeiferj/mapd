using Go = import "/go.capnp";
@0xb526ba661d550a59;
$Go.package("custom");
$Go.import("pfeifer.dev/mapd/cereal/custom");

# custom.capnp: a home for empty structs reserved for custom forks
# These structs are guaranteed to remain reserved and empty in mainline
# cereal, so use these if you want custom events in your fork.

# DO rename the structs
# DON'T change the identifier (e.g. @0x81c2f05a394cf4af)

struct CustomReserved0 @0x81c2f05a394cf4af {
}

struct CustomReserved1 @0xaedffd8f31e7b55d {
}

struct CustomReserved2 @0xf35cc4560bbf6ec2 {
}

struct CustomReserved3 @0xda96579883444c35 {
}

struct CustomReserved4 @0x80ae746ee2596b11 {
}

struct CustomReserved5 @0xa5cd762cd951a455 {
}

struct CustomReserved6 @0xf98d843bfd7004a3 {
}

struct CustomReserved7 @0xb86e6369214c01c8 {
}

struct CustomReserved8 @0xf416ec09499d9d19 {
}

struct CustomReserved9 @0xa1680744031fdb2d {
}

enum MapdInputType {
    download @0;
    setTargetLateralAccel @1;
    setSpeedLimitOffset @2;
    setSpeedLimitControl @3;
    setCurveSpeedControl @4;
    setVisionCurveSpeedControl @5;
    setLogLevel @6;
    setVtscTargetLatA @7;
    setVtscMinTargetV @8;
    reloadSettings @9;
    saveSettings @10;
    setEnableSpeed @11;
    setVtscUseEnableSpeed @12;
    setCurveUseEnableSpeed @13;
    setSpeedLimitUseEnableSpeed @14;
}

enum SpeedLimitOffsetType {
  static @0;
  percent @1;
}

struct MapdIn @0xcb9fd56c7057593a {
  type @0 :MapdInputType;
  float @1 :Float32;
  string @2 :Text;
  bool @3 :Bool;
}

enum RoadContext {
  freeway @0;
  city @1;
  unknown @2;
}

struct MapdOut @0xc2243c65e0340384 {
  wayName @0 :Text;
  wayRef @1 :Text;
  roadName @2 :Text;
  speedLimit @3 :Float32;
  nextSpeedLimit @4 :Float32;
  nextSpeedLimitDistance @5 :Float32;
  hazard @6 :Text;
  nextHazard @7 :Text;
  nextHazardDistance @8 :Float32;
  advisorySpeed @9 :Float32;
  nextAdvisorySpeed @10 :Float32;
  nextAdvisorySpeedDistance @11 :Float32;
  oneWay @12 :Bool;
  lanes @13 :UInt8;
  tileLoaded @14 :Bool;
  speedLimitOffset @15 :Float32;
  suggestedSpeed @16 :Float32;
  estimatedRoadWidth @17 :Float32;
  roadContext @18 :RoadContext;
  distanceFromWayCenter @19 :Float32;
  vtscSpeed @20 :Float32;
}

struct CustomReserved12 @0x9ccdc8676701b412 {
}

struct CustomReserved13 @0xcd96dafb67a082d0 {
}

struct CustomReserved14 @0xb057204d7deadf3f {
}

struct CustomReserved15 @0xbd443b539493bc68 {
}

struct CustomReserved16 @0xfc6241ed8877b611 {
}

struct CustomReserved17 @0xa30662f84033036c {
}

struct CustomReserved18 @0xc86a3d38d13eb3ef {
}

struct CustomReserved19 @0xa4f1eb3323f5f582 {
}
