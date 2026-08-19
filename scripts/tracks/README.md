# Recording the roads

The passes in the game are **authored** — written to match what each road is
known for, and not measured. Replacing that with the real geometry is one
command:

```bash
cd scripts
./gen_tracks.py --manifest tracks/manifest.json \
    --out ../server/internal/game/track_data.go --survey --report
```

`--survey` asks the Overpass API for each road named in `manifest.json`,
stitches the OSM ways back into one continuous line, asks an elevation service
how high every point is, and writes the result as a GPX in this directory. The
rest of the run turns those GPX files into `track_data.go`, which takes over
from the authored stand-ins automatically. Re-running without `--survey` — or
with the GPX files already present — never touches the network.

It needs to reach `overpass-api.de` and `api.opentopodata.org`. Both are free,
neither needs a key, and a sandbox with an egress policy will refuse them:

```
could not reach https://overpass-api.de/api/interpreter: Tunnel connection failed: 403 Forbidden
```

That is the only thing standing between the authored roads and measured ones.

## Doing it by hand instead

A recording you made yourself beats anything a query can produce, and any of
these is a usable source file — drop it in this directory under the `source`
name in the manifest and the importer uses it as it stands:

- **A GPX from a phone or GPS unit.** Record the drive. Elevation is worth
  having — it becomes the gradient, and gradient is most of what makes a
  descent a descent.
- **A GPX exported from a mapping or ride-logging site.** The parser matches on
  tag names rather than a namespace, so it is not fussy about which tool wrote
  it.
- **An Overpass JSON response**, saved from <https://overpass-turbo.eu>:

  ```
  [out:json][timeout:90];
  way[highway][name~"いろは坂"](36.715,139.465,36.760,139.535);
  out geom;
  ```

  `out geom;` is the part that matters: without it the response has no
  coordinates. This is exactly the query `--survey` builds from the manifest,
  so pasting it into overpass-turbo is how you check a bounding box before
  trusting what it selects.

## The bounding boxes are a starting point, not survey control

Every `osm` block in the manifest is an approximate box around the right
mountain. A box drawn slightly wrong measures the road on the other side of the
valley just as happily, so **look at the query in overpass-turbo before
believing the import**. Two things help:

- `expect` in the manifest holds the published figures for the road, and the
  importer says so out loud when its measurement lands outside them.
- `--report` prints length, corner count, tightest radius and total descent per
  road. Irohazaka descending 120 metres over 20 corners is right; descending 30
  metres over 200 corners means the trace is noisy or doubled back.

## Sections

The real roads are 6–12km. A pass in the game is about 1.5km, and its whole
pacing — race length, stamina, how long a replay takes to watch — is built on
that. So a manifest entry can name the stretch it wants:

```json
"section": [1800, 3500]
```

Metres from the start of the fetched way. The geometry inside it is measured;
where to start and stop is an authored choice, and it is written into the
track's real-world note so the claim in the API stays exactly true.

## What is measured and what is not

| | |
|---|---|
| **Measured** | length, corner radius, direction, gradient, descent, where corners begin and end |
| **From OSM tags** | width, surface — when the way carries them |
| **Still authored** | how blind a corner is, whether a crest unsettles the car, banking, which stretch of the road to use, the name in the game |

An OSM way has coordinates and **no elevation**, which is why `--survey` makes
the second request. If you import a way without one — a saved Overpass response
on its own — the importer notices, marks the track `gradient_measured: false`,
and emits no gradients at all. The game then lays the authored gradient profile
over the measured geometry, mapped by distance along the road, so the corners
are real and the slope is still someone's judgement.

A GPS trace cannot tell you that you cannot see the exit of a corner. Those
stay a judgement, and they are applied on top.

## Checking the maths

```bash
./gen_tracks.py --self-test
```

No network, and nothing real to check against, so it checks itself against
shapes whose answers are known exactly: a circle of radius R has to measure R,
five linked hairpins have to come out as five hairpins that alternate, and both
have to survive several metres of GPS noise. The fetch is covered the same way
— stand-ins for Overpass and the elevation service, served over real HTTP on
localhost, returning a road whose shape is known, with the ways shuffled, one
of them reversed and a decoy spur thrown in to be dropped.
