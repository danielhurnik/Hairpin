# Recording the roads

The passes in the game are currently **authored** — written to match what each
road is known for, and not measured. Dropping a recording of the real road in
here replaces the guesswork with geometry, and nothing else has to change.

## Getting a line down the road

Any of these produces a usable file:

- **A GPX from a phone or GPS unit.** Record the drive. Elevation is worth
  having — it becomes the gradient, and gradient is most of what makes a
  descent a descent — but the pipeline works without it.
- **A GPX exported from a mapping or ride-logging site.** Anything that exports
  a track works; the parser matches on tag names rather than a namespace, so it
  is not fussy about which tool wrote it.
- **The OSM way, via the Overpass API.** No driving required, and it comes with
  width and surface tags where mappers have added them. Run a query at
  <https://overpass-turbo.eu> and export the raw JSON:

  ```
  [out:json][timeout:90];
  way[highway](36.72,139.47,36.76,139.53);
  out geom;
  ```

  Replace the bounding box with the road you want, and narrow it with a name
  filter — `way[highway][name~"いろは坂"]` — if the box catches its neighbours.
  `out geom;` is the part that matters: without it the response has no
  coordinates.

A single continuous line in the direction the game runs it. If a GPX doubles
back or contains several laps, trim it to one pass first.

## Turning it into a track

```bash
cd scripts
./gen_tracks.py --self-test          # check the maths still recovers known shapes
./gen_tracks.py --manifest tracks/manifest.json \
    --out ../server/internal/game/track_data.go --report
```

`--report` prints what it measured — length, corner count, tightest radius,
total descent — which is the moment to sanity-check it against what you know
about the road. Irohazaka descending in 300 metres over 20 corners is right;
descending in 30 metres over 200 corners means the trace is noisy or doubled.

The generated file takes over automatically. `mergeSurveyed` prefers a measured
track over the authored one with the same ID and leaves every other road alone,
so this can be done one pass at a time.

## What is measured and what is not

| | |
|---|---|
| **Measured** | length, corner radius, direction, gradient, descent, where corners begin and end |
| **From OSM tags** | width, surface — when the way carries them |
| **Still authored** | how blind a corner is, whether a crest unsettles the car, banking, the name in the game |

A GPS trace cannot tell you that you cannot see the exit of a corner. Those
stay a judgement, and they are applied on top.
