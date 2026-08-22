#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.12,<3.14"
# dependencies = [
#   "numpy==2.5.2",
#   "pyshp==3.1.6",
#   "shapely==2.1.2",
# ]
# ///
import argparse
import copy
import hashlib
import io
import json
import math
import shapefile
import sys
import tempfile
import urllib.request
import zipfile

from itertools import pairwise, product
from pathlib import Path
from shapely import make_valid
from shapely.geometry import GeometryCollection, MultiPolygon, Polygon, box, shape
from shapely.ops import unary_union

DESCRIPTION = """Update the default menu's archive ranges from pinned boundary sources.

Run with uv run scripts/update_download_regions.py to check the menu or add
--write to update it. To adopt a newer boundary release, update its source
specification in this script, then review the generated diff.

The existing menu remains authoritative for which regions and disconnected
territories it contains. New countries and newly relevant detached territories
require an intentional menu change; this script never makes that policy choice.
"""

ARCHIVE_DEGREES = 2

REPOSITORY_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_MENU_PATH = REPOSITORY_ROOT / "settings" / "download_menu.json"
DEFAULT_CACHE_DIRECTORY = Path(tempfile.gettempdir()) / "mapd-download-region-sources"

# Natural Earth data is in the public domain.
COUNTRY_SOURCE = {
  "name": "Natural Earth 10m Admin 0 countries",
  "url": "https://raw.githubusercontent.com/nvkelso/natural-earth-vector/f1890d9f152c896d250a77557a5751a93d494776/geojson/ne_10m_admin_0_countries.geojson",
  "filename": "ne_10m_admin_0_countries.geojson",
  "sha256": "239eec57ac17f100a11e2536cffc56752c318b50ae765b0918ff7aab4ce8f255",
}

# United States Census Bureau data is in the public domain.
STATE_SOURCE = {
  "name": "Census TIGER/Line states",
  "url": "https://www2.census.gov/geo/tiger/TIGER2025/STATE/tl_2025_us_state.zip",
  "filename": "tl_2025_us_state.zip",
  "sha256": "59a220888a8d9be8117c4fcd38f542bd02d81abf0d198c78113595ad540dd957",
}

# Natural Earth uses nonstandard ISO_A2 values for these menu entries.
COUNTRY_SELECTORS = {
  "FR": ("ADM0_A3", "FRA"),
  "NO": ("ADM0_A3", "NOR"),
  "TW": ("ADM0_A3", "TWN"),
}

# mapd historically used GM for Guam; Census uses the standard GU code.
STATE_CODE_ALIASES = {"GM": "GU"}


class UpdateError(RuntimeError):
  pass


def sha256_bytes(data):
  return hashlib.sha256(data).hexdigest()


def fetch_source(specification, cache_directory):
  cache_directory.mkdir(parents=True, exist_ok=True)
  destination = cache_directory / specification["filename"]
  if destination.is_file() and sha256_bytes(destination.read_bytes()) == specification["sha256"]:
    return destination

  request = urllib.request.Request(specification["url"], headers={"User-Agent": "mapd-download-region-updater/1"})
  with urllib.request.urlopen(request, timeout=120) as response:
    content = response.read()
  if sha256_bytes(content) != specification["sha256"]:
    raise UpdateError(f"{specification['name']} no longer matches its pinned hash")
  destination.write_bytes(content)
  return destination


def polygon_components(geometry):
  if isinstance(geometry, Polygon):
    return [geometry]
  if isinstance(geometry, MultiPolygon):
    return list(geometry.geoms)
  if isinstance(geometry, GeometryCollection):
    return [polygon for part in geometry.geoms for polygon in polygon_components(part)]
  return []


def has_antimeridian_jump(geometry):
  return any(
    abs(first[0] - second[0]) > 180
    for polygon in polygon_components(geometry)
    for ring in (polygon.exterior, *polygon.interiors)
    for first, second in pairwise(ring.coords)
  )


def scoped_geometry(geometry, bounds, path):
  if not geometry.is_valid:
    geometry = make_valid(geometry)

  components = polygon_components(geometry)
  if has_antimeridian_jump(geometry):
    raise UpdateError(f"{path} has unsupported source geometry")

  latitude_archives = archive_axis(bounds["min_lat"], bounds["max_lat"])
  longitude_archives = archive_axis(bounds["min_lon"], bounds["max_lon"])
  seed = box(longitude_archives.start, latitude_archives.start, longitude_archives.stop, latitude_archives.stop)
  selected_components = [component for component in components if component.intersects(seed)]
  if not selected_components:
    raise UpdateError(f"{path} does not intersect its bounding_box")

  # Use the existing archive-aligned scope to choose components, then keep each
  # selected component whole so small bounding-box errors do not clip its archives.
  return unary_union(selected_components)


def archive_axis(minimum, maximum):
  return range(
    math.floor(minimum / ARCHIVE_DEGREES) * ARCHIVE_DEGREES,
    math.ceil(maximum / ARCHIVE_DEGREES) * ARCHIVE_DEGREES,
    ARCHIVE_DEGREES,
  )


def archives_for_bounds(bounds):
  return list(
    product(
      archive_axis(bounds["min_lat"], bounds["max_lat"]),
      archive_axis(bounds["min_lon"], bounds["max_lon"]),
    )
  )


def archives_for_geometry(geometry):
  min_longitude, min_latitude, max_longitude, max_latitude = geometry.bounds
  coordinates = product(
    archive_axis(min_latitude, max_latitude),
    archive_axis(min_longitude, max_longitude),
  )
  return [
    (latitude, longitude)
    for latitude, longitude in coordinates
    if geometry.intersects(box(longitude, latitude, longitude + ARCHIVE_DEGREES, latitude + ARCHIVE_DEGREES))
  ]


def compact_ranges(coordinates):
  sorted_coordinates = sorted(coordinates)
  ranges = []
  index = 0
  while index < len(sorted_coordinates):
    latitude, min_longitude = sorted_coordinates[index]
    max_longitude = min_longitude + ARCHIVE_DEGREES
    index += 1
    while index < len(sorted_coordinates):
      next_latitude, next_longitude = sorted_coordinates[index]
      if next_latitude != latitude or next_longitude != max_longitude:
        break
      max_longitude += ARCHIVE_DEGREES
      index += 1
    ranges.append([latitude, min_longitude, max_longitude])
  return ranges


def load_country_geometries(source_path, codes):
  features = json.loads(source_path.read_text(encoding="utf-8"))["features"]

  geometries = {}
  for code in codes:
    field, value = COUNTRY_SELECTORS.get(code, ("ISO_A2", code))
    matches = [feature for feature in features if feature.get("properties", {}).get(field) == value]
    if len(matches) != 1:
      raise UpdateError(f"nation.{code} matched {len(matches)} source features")
    geometries[code] = shape(matches[0]["geometry"])
  return geometries


def load_state_geometries(source_path, codes):
  with zipfile.ZipFile(source_path) as source_zip:
    source_names = source_zip.namelist()
    shape_name = next(name for name in source_names if name.lower().endswith(".shp"))
    database_name = next(name for name in source_names if name.lower().endswith(".dbf"))
    reader = shapefile.Reader(
      shp=io.BytesIO(source_zip.read(shape_name)),
      dbf=io.BytesIO(source_zip.read(database_name)),
    )
    source_geometries = {}
    for record in reader.iterShapeRecords():
      source_geometries[record.record.as_dict()["STUSPS"]] = shape(record.shape.__geo_interface__)

  geometries = {}
  for menu_code in codes:
    source_code = STATE_CODE_ALIASES.get(menu_code, menu_code)
    if source_code not in source_geometries:
      raise UpdateError(f"us_state.{menu_code} has no source feature")
    geometries[menu_code] = source_geometries[source_code]
  return geometries


def update_menu(menu, geometries):
  summary = {"legacy": 0, "locations": 0, "ranged": 0, "selected": 0}

  for section_name, section_geometries in geometries.items():
    entries = menu.get(section_name, {})
    for code, entry in entries.items():
      path = f"{section_name}.{code}"
      bounds = entry["bounding_box"]
      legacy_coordinates = archives_for_bounds(bounds)
      geometry = scoped_geometry(section_geometries[code], bounds, path)
      selected_coordinates = archives_for_geometry(geometry)
      ranges = compact_ranges(selected_coordinates)
      uses_archive_ranges = selected_coordinates != legacy_coordinates
      if uses_archive_ranges:
        entry["archive_ranges"] = ranges
      else:
        entry.pop("archive_ranges", None)

      summary["legacy"] += len(legacy_coordinates)
      summary["locations"] += 1
      summary["ranged"] += int(uses_archive_ranges)
      summary["selected"] += len(selected_coordinates)
  return menu, summary


def render_menu(menu, newline):
  rendered_menu = copy.deepcopy(menu)
  replacements = {}
  replacement_index = 0
  for entries in rendered_menu.values():
    for entry in entries.values():
      if "archive_ranges" not in entry:
        continue
      token = f"__MAPD_ARCHIVE_RANGES_{replacement_index}__"
      replacements[token] = entry["archive_ranges"]
      entry["archive_ranges"] = token
      replacement_index += 1

  rendered = json.dumps(rendered_menu, ensure_ascii=False, indent=2) + "\n"
  for token, ranges in replacements.items():
    rendered = rendered.replace(json.dumps(token), json.dumps(ranges))
  return rendered.replace("\n", newline)


def parse_arguments():
  parser = argparse.ArgumentParser(description=DESCRIPTION, formatter_class=argparse.RawDescriptionHelpFormatter)
  parser.add_argument("--menu", type=Path, default=DEFAULT_MENU_PATH)
  parser.add_argument("--cache-dir", type=Path, default=DEFAULT_CACHE_DIRECTORY)

  mode = parser.add_mutually_exclusive_group()
  mode.add_argument("--check", action="store_true", help="check without changing the menu (default)")
  mode.add_argument("--write", action="store_true", help="update the menu")

  return parser.parse_args()


def run(arguments):
  raw_menu = arguments.menu.read_bytes()
  newline = "\r\n" if b"\r\n" in raw_menu else "\n"
  menu = json.loads(raw_menu.decode("utf-8"))

  country_source = fetch_source(COUNTRY_SOURCE, arguments.cache_dir)
  state_source = fetch_source(STATE_SOURCE, arguments.cache_dir)

  country_geometries = load_country_geometries(country_source, menu.get("nation", {}))
  state_geometries = load_state_geometries(state_source, menu.get("us_state", {}))

  updated_menu, summary = update_menu(menu, {"nation": country_geometries, "us_state": state_geometries})
  expected = render_menu(updated_menu, newline).encode("utf-8")

  message = ("{locations} regions, {ranged} with explicit ranges; {legacy} legacy archive occurrences -> {selected} selected").format(**summary)
  if not arguments.write:
    if raw_menu != expected:
      print(f"download menu is stale ({message}); run with --write", file=sys.stderr)
      return 1
    print(f"download menu is up to date ({message})")
    return 0

  if raw_menu == expected:
    print(f"download menu already up to date ({message})")
    return 0
  arguments.menu.write_bytes(expected)
  print(f"updated {arguments.menu} ({message})")
  return 0


def main():
  try:
    return run(parse_arguments())
  except (UpdateError, OSError, ValueError, zipfile.BadZipFile) as error:
    print(f"error: {error}", file=sys.stderr)
    return 2


if __name__ == "__main__":
  raise SystemExit(main())
