# Overriding Internal Defaults

Mapd contains built in default and recommended settings, as well as a default
download menu file. The source files exist as json files in the settings folder
of the source code. They get built into the mapd binary during compile time and
are thus not directly editable. Instead, mapd will look for custom versions of
these files in the following paths:
* defaults.json: /data/openpilot/mapd\_defaults.json
* recommended.json: /data/openpilot/mapd\_recommended.json
* download\_menu.json: /data/openpilot/mapd\_download\_menu.json

## Default and Recommended Settings
Before loading custom default settings mapd will always load the
built in default settings. This ensures that any values missing from the custom
default settings will still have an appropriate default value set.

Custom defaults.json and recommended.json files should include a
`settings_version` field matching mapd's current settings schema (see
settings.md for the version and the nested json shape it expects, e.g.
`speed_limit`, `logger`, and `personalities` objects). If a custom file has an
older `settings_version` mapd automatically migrates it to the current schema
before applying it, so existing overrides do not need to be rewritten
immediately, but new overrides should target the current schema directly.

When mapd starts, it will always load the default settings before trying to load
the saved values in the params. This ensures that any new values that were not
previously saved will load with a default value. The same logic as above applies
on this first load. Mapd will load the internal defaults, then mapd will load
the custom defaults over top of the internal defaults, and finally mapd will
load the saved settings in the MapdSettings param over top of the loaded
defaults.

Recommended settings are applied overtop of the currently set values. This means
that any values not contained in the recommended settings will remain their
existing values when the recommended settings are loaded. If those values are
never set this implies that they will use the default values as the default
values are always loaded upon starting mapd.

## Download Menu
The download menu file is used for two purposes. When triggering a download, the
area names given to mapd are used to locate the appropriate bounding box from
the download menu file. The download menu file is also used to create a dynamic
menu in the mapd cli for selecting areas to download. This means that additional
areas not provided by mapd can be added as options for downloads by copying the
download\_menu.json to /data/openpilot/mapd\_download\_menu.json and then adding
any desired areas to the file. The structure is as follows:
```json
{
  "$schema": "http://json-schema.org/draft-04/schema#",
  "type": "object",
  "additionalProperties": {"$ref": "#/definitions/area_menu"},
  "definitions": {
    "area_menu": {
      "type": "object",
      "additionalProperties": {"$ref": "#/definitions/area"},
    },
    "area": {
      "type": "object",
      "properties": {
        "full_name": {
          "type": "string"
        },
        "bounding_box": {
          "type": "object",
          "properties": {
            "min_lon": {
              "type": "number"
            },
            "min_lat": {
              "type": "number"
            },
            "max_lon": {
              "type": "number"
            },
            "max_lat": {
              "type": "number"
            }
          },
          "required": [
            "min_lon",
            "min_lat",
            "max_lon",
            "max_lat"
          ]
        },
        "download_rows": {
          "type": "array",
          "items": {
            "type": "array",
            "items": {"type": "integer"},
            "minItems": 3,
            "maxItems": 3
          }
        },
        "submenu": {
          "type": "string"
        }
      },
      "required": [
        "full_name",
        "bounding_box"
      ]
    }
  }
}
```

Note the optional submenu value in an area. The submenu value allows for
chaining of the menus when requesting a download, so the submenu value should
exactly match a top level key in the main object. This value is not used by the
cli tui however, so selecting an entry with a submenu in the tui will just
result in that entry being downloaded.

An optional `download_rows` selects archives within the bounding box. Each row
is `[latitude, first_longitude, exclusive_last_longitude]`, with coordinates on
the 2-degree archive grid. For example, `[48, -124, -120]` downloads
`offline/48/-124.tar.gz` and `offline/48/-122.tar.gz`. Rows must be sorted by
latitude then longitude, must not overlap, and must stay inside the bounding
box rounded outward to the archive grid. Missing, empty, or invalid rows use
the full bounding box. Downloads and progress totals use the same selection.
Remove the rows when changing an area's bounds unless you also update its
selection. Existing menus with only bounding boxes continue to work.

### Generating archive selections

`mapd generate-download-menu` adds rows to an existing menu from local Natural
Earth 10m country and state/province GeoJSON files. This is an optional data
generation step; it does not run during map generation or require downloading
the OSM planet. Custom entries without matching geometry keep their bounding
boxes. Names, menu keys, submenus, and bounding boxes are retained.

The included rows use the [Natural Earth source](https://github.com/nvkelso/natural-earth-vector/tree/ca96624a56bd078437bca8184e78163e5039ad19)
at `ca96624a56bd078437bca8184e78163e5039ad19`. To regenerate from the repository root:

```sh
source_url=https://raw.githubusercontent.com/nvkelso/natural-earth-vector/ca96624a56bd078437bca8184e78163e5039ad19/geojson
curl -fL "$source_url/ne_10m_admin_0_countries.geojson" -o /tmp/countries.geojson
curl -fL "$source_url/ne_10m_admin_1_states_provinces.geojson" -o /tmp/states.geojson
./build/mapd generate-download-menu --countries /tmp/countries.geojson --states /tmp/states.geojson \
  --menu settings/download_menu.json --output /tmp/download_menu.json
```

Review the output before replacing a default or custom menu. The input SHA-256
hashes are `239eec57ac17f100a11e2536cffc56752c318b50ae765b0918ff7aab4ce8f255`
(countries) and `22d0e3ad85eb3e27f17cabf8ba2d50e554fbc27a87796ff891d958185da62fb5`
(states). The source data is [public domain](https://www.naturalearthdata.com/about/terms-of-use/).

Generation keeps archives intersecting outer polygon rings, including islands,
within each existing bounding box's archive grid. Intersection checks allow a
0.05-degree margin for coarse coastlines near archive edges. This retained two
coastal cells found by comparison with an independent Canada boundary; it is
not a universal boundary-accuracy guarantee. Holes and degenerate clipping
results can conservatively retain extra archives. Morocco, Somalia, and Ukraine retain their full bounding boxes
because the source's territory assignments would narrow the existing entries.
These are coarse download selections, not authoritative administrative borders;
maintain custom regions through the existing menu override.
