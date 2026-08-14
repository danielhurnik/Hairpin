#!/usr/bin/env python3
"""Turn real road geometry into Hairpin track definitions.

The tracks in `internal/game/track.go` are currently *authored*: built to match
what each pass is known for, and internally consistent under the race model,
but nobody measured them. This turns a recorded line down a real road into the
same structure, measured.

    ./gen_tracks.py --self-test
    ./gen_tracks.py --manifest tracks/manifest.json --out ../server/internal/game/track_data.go

Input is a GPX file (anything that records a track: a phone, a GPS unit, an
export from a mapping site) or an Overpass API JSON response for the road's
OSM way. Either gives a string of latitude/longitude points, ideally with
elevation.

What comes out of the geometry, and what cannot:

  measured    length, corner radius, which way each corner turns, gradient,
              total descent, where one corner ends and the next begins
  from OSM     width and surface, when the way is tagged with them
  authored     everything else — how blind a corner is, whether the crest
              actually unsettles a car, what the road is called in the game

So this does not remove judgement from the tracks. It removes guesswork from
the parts that are facts.

Stdlib only, like the rest of scripts/.
"""

import argparse
import json
import math
import os
import re
import sys
import xml.etree.ElementTree as ET

EARTH_R = 6371008.8  # mean radius, metres

# ---------------------------------------------------------------------------
# Geometry pipeline constants
#
# These are not taste. They are set by what they have to survive: consumer GPS
# noise of a few metres, and corners as tight as a 13-metre hairpin. --self-test
# measures both, so changing one of these has an immediate honest answer.
# ---------------------------------------------------------------------------

RESAMPLE_M = 4.0        # even spacing along the line before anything else
SMOOTH_WINDOW = 3       # coordinate smoothing, in samples, to take the jitter off
CURVATURE_CHORD_M = 12.0  # how far ahead and behind to look when measuring a bend
MEDIAN_WINDOW = 3       # median filter on curvature; preserves corner entry/exit
STRAIGHT_RADIUS_M = 130.0  # anything gentler than this is a straight
FIT_TOL_FLOOR_M = 0.25  # smallest fit tolerance, so a flawless trace is still workable
MIN_SEGMENT_M = 22.0    # runs shorter than this get folded into their neighbour
FIT_MIN_HALF_M = 10.0   # shortest half-window for the circle fit — a tight hairpin
FIT_MAX_HALF_M = 70.0   # longest — a gentle sweeper needs a lot of road to measure


# ---------------------------------------------------------------------------
# Reading a line
# ---------------------------------------------------------------------------

def read_gpx(path):
    """Every trackpoint in a GPX, in order, as (lat, lon, ele|None)."""
    tree = ET.parse(path)
    pts = []
    # GPX is namespaced and the namespace varies by recorder, so match on the
    # local tag name rather than assuming one.
    for el in tree.iter():
        if el.tag.rsplit('}', 1)[-1] not in ('trkpt', 'rtept', 'wpt'):
            continue
        try:
            lat = float(el.attrib['lat'])
            lon = float(el.attrib['lon'])
        except (KeyError, ValueError):
            continue
        ele = None
        for child in el:
            if child.tag.rsplit('}', 1)[-1] == 'ele' and child.text:
                try:
                    ele = float(child.text)
                except ValueError:
                    pass
        pts.append((lat, lon, ele))
    return pts


def read_overpass(path):
    """Points and tags from an Overpass JSON response.

    Expects a query run with `out geom;` so each way carries its coordinates.
    Ways are stitched in the order they appear, which is why the query should
    ask for one road at a time.
    """
    with open(path) as fh:
        doc = json.load(fh)
    pts, tags = [], {}
    for el in doc.get('elements', []):
        if el.get('type') != 'way':
            continue
        tags.update(el.get('tags') or {})
        for node in el.get('geometry') or []:
            pts.append((node['lat'], node['lon'], None))
    return pts, tags


# ---------------------------------------------------------------------------
# Projection and resampling
# ---------------------------------------------------------------------------

def project(points):
    """Latitude/longitude to local metres.

    Equirectangular about the middle of the road. Over the few kilometres of a
    mountain pass the error against a proper projection is centimetres, and it
    keeps the maths readable.
    """
    if not points:
        return []
    lat0 = sum(p[0] for p in points) / len(points)
    lon0 = sum(p[1] for p in points) / len(points)
    k = math.cos(math.radians(lat0))
    out = []
    for lat, lon, ele in points:
        x = math.radians(lon - lon0) * EARTH_R * k
        y = math.radians(lat - lat0) * EARTH_R
        out.append((x, y, ele))
    return out


def resample(pts, step=RESAMPLE_M):
    """Even spacing along the line.

    GPS points arrive at whatever rate the device felt like, which means a
    curvature estimate from raw samples measures how fast you were driving as
    much as how tight the corner was.
    """
    if len(pts) < 2:
        return list(pts)
    out = [pts[0]]
    carry = 0.0
    for i in range(len(pts) - 1):
        (x0, y0, e0), (x1, y1, e1) = pts[i], pts[i + 1]
        seg = math.hypot(x1 - x0, y1 - y0)
        if seg <= 1e-9:
            continue
        t = carry
        while t + step <= seg:
            t += step
            f = t / seg
            ele = None
            if e0 is not None and e1 is not None:
                ele = e0 + (e1 - e0) * f
            out.append((x0 + (x1 - x0) * f, y0 + (y1 - y0) * f, ele))
        carry = t - seg
    return out


def smooth(pts, window=SMOOTH_WINDOW):
    """Moving average over the coordinates to take the jitter off.

    Deliberately gentle: a 13-metre hairpin only occupies a handful of samples,
    and smoothing hard enough to erase GPS noise also erases the corner.
    """
    if window <= 1 or len(pts) < window:
        return list(pts)
    half = window // 2
    out = []
    for i in range(len(pts)):
        lo, hi = max(0, i - half), min(len(pts), i + half + 1)
        n = hi - lo
        sx = sum(p[0] for p in pts[lo:hi]) / n
        sy = sum(p[1] for p in pts[lo:hi]) / n
        eles = [p[2] for p in pts[lo:hi] if p[2] is not None]
        se = sum(eles) / len(eles) if eles else None
        out.append((sx, sy, se))
    return out


# ---------------------------------------------------------------------------
# Curvature
# ---------------------------------------------------------------------------

def signed_curvature(pts, chord=CURVATURE_CHORD_M, step=RESAMPLE_M):
    """Menger curvature at every sample, signed by which way the road turns.

    For three points the circle through them has radius |AB|·|BC|·|CA| / 4·area,
    and curvature is its reciprocal. Taking A and C a fixed *distance* either
    side of B rather than adjacent samples is what makes this survive real
    data: a 12-metre chord through a 15-metre hairpin is a big, unambiguous
    triangle, while three adjacent 4-metre samples are mostly noise.

    Sign is the z component of the cross product: positive turns left.
    """
    n = len(pts)
    look = max(1, int(round(chord / step)))
    out = [0.0] * n
    for i in range(n):
        a, b, c = pts[max(0, i - look)], pts[i], pts[min(n - 1, i + look)]
        abx, aby = b[0] - a[0], b[1] - a[1]
        bcx, bcy = c[0] - b[0], c[1] - b[1]
        cax, cay = a[0] - c[0], a[1] - c[1]
        la = math.hypot(abx, aby)
        lb = math.hypot(bcx, bcy)
        lc = math.hypot(cax, cay)
        if la < 1e-6 or lb < 1e-6 or lc < 1e-6:
            continue
        cross = abx * bcy - aby * bcx
        area2 = abs(cross)
        if area2 < 1e-9:
            continue
        kappa = 2 * area2 / (la * lb * lc)
        out[i] = kappa if cross > 0 else -kappa
    return out


def fit_circle(window):
    """Least-squares circle through a run of points.

    Returns (radius, cx, cy, rms_residual). Algebraic (Kasa) fit: the circle
    x^2 + y^2 + Dx + Ey + F = 0 is linear in (D, E, F), so this is a 3x3
    normal-equation solve and needs no iteration.

    The point of fitting rather than taking three samples is noise rejection.
    Menger curvature uses exactly three points however many are available, so
    error in any one of them lands directly on the answer; a fit over thirty
    points averages it down.

    The residual is what makes the window selectable. It measures how well this
    stretch of road actually IS a circle, which is a different question from
    how noisy the samples are, and it is the only signal that can tell a
    genuine 120-metre sweeper from a hairpin being looked at through too long
    a lens.
    """
    n = len(window)
    if n < 4:
        return 0.0, 0.0, 0.0, 1e9
    mx = sum(p[0] for p in window) / n
    my = sum(p[1] for p in window) / n

    s11 = s12 = s22 = s1z = s2z = 0.0
    for px, py in ((p[0] - mx, p[1] - my) for p in window):
        z = px * px + py * py
        s11 += px * px
        s12 += px * py
        s22 += py * py
        s1z += px * z
        s2z += py * z

    det = s11 * s22 - s12 * s12
    if abs(det) < 1e-9:
        return 0.0, 0.0, 0.0, 1e9  # collinear: a straight
    cx = (s1z * s22 - s2z * s12) / (2 * det)
    cy = (s2z * s11 - s1z * s12) / (2 * det)
    r = math.sqrt(max(cx * cx + cy * cy + (s11 + s22) / n, 0.0))
    if r <= 0:
        return 0.0, 0.0, 0.0, 1e9

    acc = 0.0
    for px, py in ((p[0] - mx, p[1] - my) for p in window):
        acc += (math.hypot(px - cx, py - cy) - r) ** 2
    return r, cx + mx, cy + my, math.sqrt(acc / n)


# Half-window lengths tried at every sample, longest first. The longest that
# still describes the road is the one used.
FIT_SCALES_M = (100.0, 70.0, 50.0, 34.0, 24.0, 16.0, 11.0)


def fitted_curvature(pts, step=RESAMPLE_M):
    """Signed curvature by circle fit, at the longest scale that still fits.

    How much road you need to look at depends on how much it is bending, and
    you do not know that until you have looked. Two earlier attempts failed in
    instructive ways, and both are worth keeping written down:

    Guessing a scale from a rough reading and refining it is a trap. On noisy
    data the rough reading is wrong, a wrong reading picks a short window, and a
    short window confirms it — a 60-metre sweeper came back as 16 metres however
    many times it iterated.

    Judging the fit against an absolute tolerance cannot serve both inputs. Set
    it loose enough for a phone in a wooded valley and, on a clean trace, a
    window spanning a hairpin AND its exit straight passes as one gentle
    39-metre curve; five linked hairpins smear into three. Set it tight and
    every honest fit on a real recording is rejected.

    Estimating the trace's noise globally does not work either, because
    smoothing correlates neighbouring samples: a short window fits almost
    anything well, so its residual reads 0.68m on a trace whose correct
    long-window fit leaves 1.49m behind.

    So the trace is judged against itself. Measure what the shortest window
    leaves behind across the whole road — that is this recording at its most
    forgiving — and accept the longest scale that is not dramatically worse. A
    correct arc fits about as well at every scale. A window that has swallowed
    the next corner does not.
    """
    n = len(pts)
    shortest = max(2, int(round(FIT_SCALES_M[-1] / step)))

    # The bar is set once for the whole trace, from the median of what the
    # shortest window leaves behind. Per-sample it is far too jumpy — seven
    # points give a noisy estimate of noise, and a low reading rejects the
    # correct long fit and collapses that sample to a short one.
    shorts = []
    for i in range(0, n, 3):
        lo, hi = max(0, i - shortest), min(n, i + shortest + 1)
        if hi - lo < 6:
            continue
        _, _, _, resid = fit_circle(pts[lo:hi])
        if resid < 1e8:
            shorts.append(resid)
    shorts.sort()
    base_resid = shorts[len(shorts) // 2] if shorts else 0.0
    tol = max(3.0 * base_resid, FIT_TOL_FLOOR_M)

    out = [0.0] * n
    for i in range(n):
        chosen = None
        for half_m in FIT_SCALES_M:
            half = max(2, int(round(half_m / step)))
            a, b = max(0, i - half), min(n, i + half + 1)
            if b - a < 6:
                continue
            r, cx, cy, resid = fit_circle(pts[a:b])
            if r <= 0 or r > 8000:
                continue
            if chosen is None:
                chosen = (r, cx, cy, a, b)  # fallback: the shortest usable
            if resid <= tol:
                chosen = (r, cx, cy, a, b)
                break
        if chosen is None:
            continue
        r, cx, cy, a, b = chosen
        ax, ay = pts[a][0], pts[a][1]
        bx, by = pts[b - 1][0], pts[b - 1][1]
        tx, ty = bx - ax, by - ay
        side = tx * (cy - ay) - ty * (cx - ax)
        out[i] = (1.0 / r) if side > 0 else -(1.0 / r)
    return median_filter(out)


def median_filter(xs, window=MEDIAN_WINDOW):
    """Median rather than mean: it holds the edge where a corner starts."""
    if window <= 1 or len(xs) < window:
        return list(xs)
    half = window // 2
    out = []
    for i in range(len(xs)):
        lo, hi = max(0, i - half), min(len(xs), i + half + 1)
        out.append(sorted(xs[lo:hi])[(hi - lo) // 2])
    return out


# ---------------------------------------------------------------------------
# Segmentation
# ---------------------------------------------------------------------------

def segment(pts, kappa, step=RESAMPLE_M):
    """Group the line into corners and straights.

    A sample is 'corner' when the road is bending tighter than
    STRAIGHT_RADIUS_M; runs of the same kind become one piece of road. Short
    runs are folded into whichever neighbour they interrupted, so a wobble
    halfway down a straight does not become a corner in its own right.
    """
    straight_k = 1.0 / STRAIGHT_RADIUS_M

    def kind_of(k):
        if abs(k) < straight_k:
            return 0
        return 1 if k > 0 else -1

    kinds = [kind_of(k) for k in kappa]

    runs = []
    start = 0
    for i in range(1, len(kinds) + 1):
        if i == len(kinds) or kinds[i] != kinds[start]:
            runs.append([start, i, kinds[start]])
            start = i

    def coalesce(rs):
        """Join neighbours that are the same kind of road.

        Folding a short run away can leave the two runs either side of it
        adjacent and identical, and leaving them separate splits one hairpin
        into two — which is how five linked hairpins came out as thirteen
        pieces of road, each reading looser than the corner really is.
        """
        out = [rs[0]]
        for r in rs[1:]:
            if r[2] == out[-1][2]:
                out[-1][1] = r[1]
            else:
                out.append(r)
        return out

    runs = coalesce(runs)

    # Fold anything too short to be a real piece of road into a neighbour.
    changed = True
    while changed and len(runs) > 1:
        changed = False
        for idx, (a, b, _) in enumerate(runs):
            if (b - a) * step >= MIN_SEGMENT_M:
                continue
            if idx == 0:
                runs[1][0] = a
            elif idx == len(runs) - 1:
                runs[-2][1] = b
            else:
                # Give it to the longer neighbour.
                before = runs[idx - 1][1] - runs[idx - 1][0]
                after = runs[idx + 1][1] - runs[idx + 1][0]
                if before >= after:
                    runs[idx - 1][1] = b
                else:
                    runs[idx + 1][0] = a
            runs.pop(idx)
            runs = coalesce(runs)
            changed = True
            break

    out = []
    for a, b, kind in runs:
        length = (b - a) * step
        if length <= 0:
            continue
        # Curvature over the middle of the corner, trimmed at both ends.
        #
        # Entry and exit are where one piece of road blurs into the next, and
        # averaging them in makes every corner read looser than it is. On five
        # linked hairpins — which is the whole character of Akina — that turned
        # R16 into R29 and swallowed two of the five outright.
        #
        # Median rather than mean, so a single bad sample cannot open a hairpin
        # out either.
        ks = sorted(abs(k) for k in trimmed(kappa[a:b]))
        mid_k = ks[len(ks) // 2] if ks else 0.0
        radius = 0.0 if kind == 0 or mid_k <= 1e-9 else 1.0 / mid_k

        e0, e1 = pts[a][2], pts[min(b, len(pts) - 1)][2]
        gradient = 0.0
        if e0 is not None and e1 is not None and length > 0:
            gradient = (e1 - e0) / length * 100.0

        out.append({
            'radius': radius,
            'length': length,
            'gradient': gradient,
            'dir': kind,
            'start': a,
            'end': b,
        })
    return out


def trimmed(xs, fraction=0.2):
    """The middle of a run, with the transitions at each end dropped."""
    if len(xs) < 5:
        return list(xs)
    cut = max(1, int(len(xs) * fraction))
    return xs[cut:len(xs) - cut]


def build_track(points, tags=None, meta=None):
    """The whole pipeline, from raw points to segments."""
    meta = meta or {}
    tags = tags or {}
    has_ele = any(p[2] is not None for p in points)
    pts = smooth(resample(project(points)))
    if len(pts) < 8:
        raise SystemExit('not enough points to work with (%d after resampling)' % len(pts))
    kappa = fitted_curvature(pts)
    segs = segment(pts, kappa)

    width = meta.get('width')
    if width is None:
        width = osm_width(tags)
    surface = meta.get('surface') or osm_surface(tags)

    for s in segs:
        s['width'] = width
        s['surface'] = surface
        s['gradient_measured'] = has_ele
    return pts, kappa, segs


def osm_width(tags):
    """Carriageway width from OSM tags, or a sensible mountain-road default."""
    for key in ('width', 'est_width'):
        if key in tags:
            m = re.match(r'\s*([0-9.]+)', str(tags[key]))
            if m:
                return float(m.group(1))
    if 'lanes' in tags:
        try:
            return max(2.6 * int(tags['lanes']), 3.0)
        except ValueError:
            pass
    return 6.0


def osm_surface(tags):
    """OSM surface tags onto the four the game models."""
    s = str(tags.get('surface', '')).lower()
    if s in ('asphalt', 'paved', 'concrete'):
        smoothness = str(tags.get('smoothness', '')).lower()
        if smoothness in ('bad', 'very_bad', 'horrible', 'very_horrible'):
            return 'SurfaceBroken'
        if smoothness in ('intermediate',):
            return 'SurfacePatched'
        return 'SurfaceSmooth'
    if s in ('chipseal', 'compacted', 'gravel', 'fine_gravel'):
        return 'SurfaceCoarse'
    if s in ('cobblestone', 'sett', 'unpaved', 'ground', 'dirt'):
        return 'SurfaceBroken'
    return 'SurfaceCoarse'


# ---------------------------------------------------------------------------
# Emitting Go
# ---------------------------------------------------------------------------

def go_track(meta, segs):
    name_for = meta.get('segment_names') or []
    lines = []
    lines.append('\t{')
    lines.append('\t\tID: %s, Name: %s, Source: SourceSurveyed,' % (
        go_str(meta['id']), go_str(meta['name'])))
    if meta.get('downhill'):
        lines.append('\t\tDownhill: true,')
    lines.append('\t\tFlavour: %s,' % go_str(meta.get('flavour', '')))
    measured = bool(segs) and segs[0].get('gradient_measured')
    lines.append('\t\tGradientMeasured: %s,' % ('true' if measured else 'false'))
    if meta.get('real_world'):
        lines.append('\t\tRealWorld: %s,' % go_str(meta['real_world']))
    lines.append('\t\tSegments: []Segment{')
    for i, s in enumerate(segs):
        nm = name_for[i] if i < len(name_for) else default_name(s, i, len(segs))
        parts = ['Name: %s' % go_str(nm)]
        if s['radius'] > 0:
            parts.append('Radius: %.0f' % s['radius'])
        parts.append('Length: %.0f' % s['length'])
        parts.append('Width: %.1f' % s['width'])
        if s.get('gradient_measured'):
            parts.append('Gradient: %.1f' % s['gradient'])
        parts.append('Surface: %s' % s['surface'])
        if s['dir']:
            parts.append('Dir: %d' % s['dir'])
        lines.append('\t\t\t{%s},' % ', '.join(parts))
    lines.append('\t\t},')
    lines.append('\t},')
    return '\n'.join(lines)


def default_name(s, i, n):
    if s['radius'] == 0:
        return 'Straight %d' % (i + 1)
    side = 'Left' if s['dir'] > 0 else 'Right'
    if s['radius'] < 24:
        return 'Hairpin %s %d' % (side, i + 1)
    if s['radius'] < 65:
        return 'Turn %s %d' % (side, i + 1)
    return 'Sweeper %s %d' % (side, i + 1)


def go_str(s):
    return '"%s"' % s.replace('\\', '\\\\').replace('"', '\\"')


HEADER = '''// Code generated by scripts/gen_tracks.py. DO NOT EDIT.
//
// Every radius, length, gradient and direction below was measured from a
// recorded line down the real road rather than authored. Width and surface
// come from OSM tags where the way carried them. Everything the geometry
// cannot know — how blind a corner is, whether a crest unsettles a car — is
// still a judgement, and is applied on top in track.go.

package game

var surveyedTracks = []TrackDef{
%s
}
'''


# ---------------------------------------------------------------------------
# Self-test
#
# There is no real GPX in the repo, so the maths is checked against shapes
# whose answers are known exactly. A circle of radius 60 has to come back as
# radius 60 — including when several metres of GPS noise are thrown at it,
# which is the case that decides whether any of this survives contact with a
# real recording.
# ---------------------------------------------------------------------------

class Path:
    """A road built piece by piece, continuous in position AND heading.

    This exists because an earlier fixture translated each arc onto the end of
    the last one without rotating it to match the direction of travel. That
    produces a zigzag of disconnected arcs with a kink at every join, and the
    pipeline duly reported wild alternating curvature — which looked for a
    while like a bug in the measurement rather than in the road being measured.
    """

    def __init__(self, lat0=36.75, lon0=139.5, ele0=1000.0, heading_deg=0.0, seed=11):
        self.lat0, self.lon0 = lat0, lon0
        self.k = math.cos(math.radians(lat0))
        self.x = self.y = 0.0
        self.h = math.radians(heading_deg)
        self.ele = ele0
        self.pts = []
        self.rnd = _Rand(seed)
        self._emit()

    def _emit(self, noise=0.0):
        x, y = self.x, self.y
        if noise:
            x += self.rnd.normal() * noise
            y += self.rnd.normal() * noise
        self.pts.append((self.lat0 + math.degrees(y / EARTH_R),
                         self.lon0 + math.degrees(x / (EARTH_R * self.k)),
                         self.ele))

    def straight(self, length, gradient=0.0, step=3.0, noise=0.0):
        n = max(1, int(length / step))
        dx, dy = math.sin(self.h), math.cos(self.h)
        for _ in range(n):
            d = length / n
            self.x += dx * d
            self.y += dy * d
            self.ele += gradient / 100.0 * d
            self._emit(noise)
        return self

    def arc(self, radius, sweep_deg, gradient=0.0, step=3.0, noise=0.0):
        """Turn through sweep_deg at the given radius. Positive sweeps left."""
        sweep = math.radians(sweep_deg)
        sign = 1.0 if sweep >= 0 else -1.0
        # Centre sits perpendicular to the direction of travel, on the inside.
        cx = self.x - sign * radius * math.cos(self.h)
        cy = self.y + sign * radius * math.sin(self.h)
        arc_len = abs(sweep) * radius
        n = max(2, int(arc_len / step))
        start = math.atan2(self.y - cy, self.x - cx)
        for i in range(1, n + 1):
            a = start + sweep * (i / n)
            self.x = cx + radius * math.cos(a)
            self.y = cy + radius * math.sin(a)
            self.ele += gradient / 100.0 * (arc_len / n)
            self._emit(noise)
        self.h -= sweep
        return self

    def points(self):
        return list(self.pts)


def synth_arc(radius, sweep_deg, start_deg=0.0, lat0=36.75, lon0=139.5,
              ele0=1000.0, gradient_pct=0.0, step_m=3.0, noise_m=0.0, seed=1):
    """Points along a circular arc, as latitude/longitude."""
    rnd = _Rand(seed)
    k = math.cos(math.radians(lat0))
    arc_len = abs(math.radians(sweep_deg) * radius)
    n = max(3, int(arc_len / step_m))
    pts = []
    for i in range(n + 1):
        t = i / n
        ang = math.radians(start_deg + sweep_deg * t)
        x = radius * math.sin(ang)
        y = radius * (1 - math.cos(ang))
        if noise_m:
            x += rnd.normal() * noise_m
            y += rnd.normal() * noise_m
        lat = lat0 + math.degrees(y / EARTH_R)
        lon = lon0 + math.degrees(x / (EARTH_R * k))
        ele = ele0 + gradient_pct / 100.0 * (arc_len * t)
        pts.append((lat, lon, ele))
    return pts


def synth_line(length_m, heading_deg=0.0, lat0=36.75, lon0=139.5,
               ele0=1000.0, gradient_pct=0.0, step_m=3.0, noise_m=0.0, seed=2):
    rnd = _Rand(seed)
    k = math.cos(math.radians(lat0))
    n = max(2, int(length_m / step_m))
    pts = []
    for i in range(n + 1):
        d = length_m * i / n
        x = d * math.sin(math.radians(heading_deg))
        y = d * math.cos(math.radians(heading_deg))
        if noise_m:
            x += rnd.normal() * noise_m
            y += rnd.normal() * noise_m
        lat = lat0 + math.degrees(y / EARTH_R)
        lon = lon0 + math.degrees(x / (EARTH_R * k))
        pts.append((lat, lon, ele0 + gradient_pct / 100.0 * d))
    return pts


class _Rand:
    """A tiny deterministic normal generator, so self-test results repeat."""

    def __init__(self, seed):
        self.s = seed * 2654435761 % (2 ** 32)

    def uniform(self):
        self.s = (1103515245 * self.s + 12345) % (2 ** 31)
        return self.s / (2 ** 31)

    def normal(self):
        u1 = max(self.uniform(), 1e-9)
        u2 = self.uniform()
        return math.sqrt(-2 * math.log(u1)) * math.cos(2 * math.pi * u2)


def self_test():
    failures = []

    def check(name, ok, detail):
        status = 'ok  ' if ok else 'FAIL'
        print('  [%s] %-46s %s' % (status, name, detail))
        if not ok:
            failures.append(name)

    print('curvature recovery — a known circle must come back as itself')
    for radius in (15, 30, 60, 120):
        pts = smooth(resample(project(synth_arc(radius, 120))))
        kappa = fitted_curvature(pts)
        mid = kappa[len(kappa) // 4: 3 * len(kappa) // 4]
        got = 1.0 / (sum(abs(k) for k in mid) / len(mid))
        err = abs(got - radius) / radius
        check('radius %dm' % radius, err < 0.08, 'measured %.1fm (%.1f%% out)' % (got, err * 100))

    print('\nthe same, with GPS noise thrown at it')
    for radius, noise in ((20, 1.5), (60, 2.5), (120, 2.5)):
        pts = smooth(resample(project(synth_arc(radius, 150, noise_m=noise, seed=7))))
        kappa = fitted_curvature(pts)
        mid = kappa[len(kappa) // 4: 3 * len(kappa) // 4]
        got = 1.0 / (sum(abs(k) for k in mid) / len(mid))
        err = abs(got - radius) / radius
        check('radius %dm with %.1fm noise' % (radius, noise), err < 0.15,
              'measured %.1fm (%.1f%% out)' % (got, err * 100))

    print('\nsegmentation')
    _, _, segs = build_track(synth_line(400), meta={'width': 6.0})
    check('a straight is one straight', len(segs) == 1 and segs[0]['radius'] == 0,
          '%d segment(s), radius %.0f' % (len(segs), segs[0]['radius'] if segs else -1))

    # An S-bend has to come out as two corners that disagree about which way
    # they go.
    sbend = Path().arc(45, 110).arc(45, -110).points()
    _, _, segs = build_track(sbend, meta={'width': 6.0})
    corners = [x for x in segs if x['radius'] > 0]
    dirs = {x['dir'] for x in corners}
    check('an S-bend is two corners, opposite ways', len(corners) == 2 and len(dirs) == 2,
          '%d corners, directions %s' % (len(corners), sorted(dirs)))

    print('\ngradient')
    _, _, segs = build_track(synth_line(500, gradient_pct=-8.0), meta={'width': 6.0})
    got = segs[0]['gradient']
    check('a -8%% descent reads as -8%%', abs(got + 8.0) < 0.5, 'measured %.2f%%' % got)

    print('\nlength')
    _, _, segs = build_track(synth_line(600), meta={'width': 6.0})
    total = sum(s['length'] for s in segs)
    check('600m of road measures 600m', abs(total - 600) < 25, 'measured %.0fm' % total)

    print('\nOSM tag mapping')
    check('width from a lanes tag', osm_width({'lanes': '2'}) == 5.2, '%.1fm' % osm_width({'lanes': '2'}))
    check('width from an explicit tag', osm_width({'width': '5.5 m'}) == 5.5, '%.1fm' % osm_width({'width': '5.5 m'}))
    check('rough asphalt is broken', osm_surface({'surface': 'asphalt', 'smoothness': 'bad'}) == 'SurfaceBroken',
          osm_surface({'surface': 'asphalt', 'smoothness': 'bad'}))

    print('\nlinked hairpins — the thing these roads are actually made of')
    p = Path()
    for i in range(5):
        p.arc(16, 165 if i % 2 == 0 else -165, gradient=-9)
        p.straight(45, gradient=-9)
    _, _, segs = build_track(p.points(), meta={'width': 6.0})
    corners = [x for x in segs if x['radius'] > 0]
    radii = [x['radius'] for x in corners]
    check('five linked hairpins resolve as five corners', len(corners) == 5,
          'found %d' % len(corners))
    check('each measures about 16m',
          radii and all(abs(r - 16) / 16 < 0.12 for r in radii),
          'R %s' % ', '.join('%.1f' % r for r in radii))
    check('and they alternate direction',
          len(corners) == 5 and all(corners[i]['dir'] != corners[i + 1]['dir']
                                    for i in range(len(corners) - 1)),
          '%s' % ''.join('LR'[x['dir'] < 0] for x in corners))

    print('\nelevation')
    flat = [(la, lo, None) for la, lo, _ in Path().arc(30, 120).straight(200).points()]
    _, _, segs = build_track(flat, meta={'width': 6.0})
    check('a source with no elevation says so', not segs[0]['gradient_measured'],
          'gradient_measured=%s' % segs[0]['gradient_measured'])
    go = go_track({'id': 'x', 'name': 'X', 'flavour': ''}, segs)
    check('and the Go it emits carries no gradient at all',
          'GradientMeasured: false' in go and 'Gradient:' not in go,
          'ok')
    withele = Path(ele0=900).arc(30, 120, gradient=-7).straight(200, gradient=-7).points()
    _, _, segs = build_track(withele, meta={'width': 6.0})
    check('a source with elevation is measured', segs[0]['gradient_measured'],
          '%.1f%% on the first corner' % segs[0]['gradient'])

    print('\nend to end — a GPX in, Go out')
    import tempfile
    joined = Path(ele0=1000).arc(18, 170, gradient=-9).straight(300, gradient=-6).points()
    gpx = ['<?xml version="1.0"?><gpx version="1.1"><trk><trkseg>']
    for la, lo, e in joined:
        gpx.append('<trkpt lat="%.8f" lon="%.8f"><ele>%.2f</ele></trkpt>' % (la, lo, e))
    gpx.append('</trkseg></trk></gpx>')
    with tempfile.NamedTemporaryFile('w', suffix='.gpx', delete=False) as fh:
        fh.write(''.join(gpx))
        gpx_path = fh.name
    reread = read_gpx(gpx_path)
    os.unlink(gpx_path)
    check('the GPX reads back', len(reread) == len(joined),
          '%d of %d points' % (len(reread), len(joined)))
    _, _, segs = build_track(reread, meta={'width': 6.4})
    corners = [x for x in segs if x['radius'] > 0]
    straights = [x for x in segs if x['radius'] == 0]
    check('a hairpin into a straight is two pieces of road',
          len(corners) >= 1 and len(straights) >= 1,
          '%d corner(s), %d straight(s)' % (len(corners), len(straights)))
    if corners:
        check('the hairpin measures about 18m', abs(corners[0]['radius'] - 18) / 18 < 0.15,
              'R%.1f' % corners[0]['radius'])
    go = go_track({'id': 'selftest', 'name': 'Self Test', 'flavour': 'x'}, segs)
    check('the Go it emits looks like Go',
          go.count('{') == go.count('}') and 'Segments: []Segment{' in go,
          '%d lines' % len(go.splitlines()))

    print()
    if failures:
        print('%d check(s) failed: %s' % (len(failures), ', '.join(failures)))
        return 1
    print('all checks passed — the pipeline recovers known geometry')
    return 0


# ---------------------------------------------------------------------------

def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument('--self-test', action='store_true',
                    help='verify the geometry maths against known shapes and exit')
    ap.add_argument('--manifest', help='JSON describing each track and its source file')
    ap.add_argument('--out', help='Go file to write')
    ap.add_argument('--report', action='store_true', help='print what was measured')
    args = ap.parse_args()

    if args.self_test:
        sys.exit(self_test())

    if not args.manifest:
        ap.error('need --manifest (or --self-test)')

    with open(args.manifest) as fh:
        manifest = json.load(fh)
    base = os.path.dirname(os.path.abspath(args.manifest))

    blocks = []
    for meta in manifest['tracks']:
        src = os.path.join(base, meta['source'])
        tags = {}
        if src.endswith('.json'):
            points, tags = read_overpass(src)
        else:
            points = read_gpx(src)
        if not points:
            raise SystemExit('no points in %s' % src)
        pts, _, segs = build_track(points, tags, meta)

        if args.report:
            if not segs[0].get('gradient_measured'):
                print('  ! %s has no elevation — gradients will be borrowed from '
                      'the authored stand-in' % meta['name'])
            total = sum(s['length'] for s in segs)
            drop = sum(-s['gradient'] / 100 * s['length'] for s in segs if s['gradient'] < 0)
            corners = [s for s in segs if s['radius'] > 0]
            tightest = min((s['radius'] for s in corners), default=0)
            print('%-28s %5.0fm  %2d corners  tightest R%.0f  descent %.0fm'
                  % (meta['name'], total, len(corners), tightest, drop))

        blocks.append(go_track(meta, segs))

    out = HEADER % '\n'.join(blocks)
    if args.out:
        with open(args.out, 'w') as fh:
            fh.write(out)
        print('wrote %s' % args.out)
    else:
        print(out)


if __name__ == '__main__':
    main()
