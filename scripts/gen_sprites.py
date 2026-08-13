#!/usr/bin/env python3
"""Top-down 8-bit cars for Hairpin. Nose points up.

Built from a per-row width profile so noses and tails taper properly, then
cabin, glass, wheels and lights are stamped on top. Everything is parameterised
so the seven chassis stay consistent with each other.
"""
import struct, zlib

W, H = 20, 34
OUT = '/tmp/claude-0/-home-user-Hairpin/5ed2e085-506e-5582-aa22-660b02c6f288/scratchpad'

PALETTE = {
    '.': None,
    'O': (96, 106, 124),     # outline — must read against a black roof AND a dark background
    '1': (232, 232, 228),   # primary (livery)
    '2': (26, 28, 34),      # secondary (livery) — roof
    '3': (150, 152, 158),   # shade
    'G': (96, 124, 156),    # glass
    'T': (24, 25, 30),      # tyre — darkest thing on the sprite
    'H': (255, 214, 130),   # headlight
    'B': (224, 52, 75),     # tail light
}


def blank():
    return [['.'] * W for _ in range(H)]


def put(g, x, y, ch):
    if 0 <= x < W and 0 <= y < H:
        g[y][x] = ch


def row_span(g, y, half, ch, outline=True):
    """Fill a horizontal span centred on the sprite."""
    cx = W // 2
    x0, x1 = cx - half, cx + half - 1
    for x in range(x0, x1 + 1):
        put(g, x, y, ch)
    if outline:
        put(g, x0 - 1, y, 'O')
        put(g, x1 + 1, y, 'O')
    return x0, x1


def make_car(p):
    g = blank()
    body = p['body']              # list of (y, half-width)
    nose_y = body[0][0]
    tail_y = body[-1][0]

    # --- body shell ---------------------------------------------------------
    for (y, half) in body:
        row_span(g, y, half, '1')
    # cap the nose and tail with outline
    top_half = body[0][1]
    bot_half = body[-1][1]
    row_span(g, nose_y - 1, top_half, 'O', outline=False)
    row_span(g, tail_y + 1, bot_half, 'O', outline=False)

    # --- cabin: roof in the livery's secondary, glass front and back --------
    c0, c1 = p['cabin']
    cab_half = p['cabin_half']
    for y in range(c0, c1 + 1):
        row_span(g, y, cab_half, '2', outline=False)
    # Outline the cabin: the secondary livery colour is near-black on the
    # starter paint, and without this ring the roof reads as a hole.
    row_span(g, c0 - 1, cab_half, 'O', outline=False)
    row_span(g, c1 + 1, cab_half, 'O', outline=False)
    for y in range(c0 - 1, c1 + 2):
        put(g, W // 2 - cab_half, y, 'O')
        put(g, W // 2 + cab_half - 1, y, 'O')
    # windscreen (front of cabin) and rear screen
    for i in range(p['screen']):
        row_span(g, c0 + i, cab_half - (0 if i else 1), 'G', outline=False)
    for i in range(p['rear_screen']):
        row_span(g, c1 - i, cab_half - (0 if i else 1), 'G', outline=False)

    # --- bonnet / boot shading so the shape reads from above ----------------
    for y in range(nose_y + 1, c0 - 1):
        put(g, W // 2 - 1, y, '3')
        put(g, W // 2, y, '3')

    # --- wheels: sit proud of the body on both sides ------------------------
    half = max(h for _, h in body)
    for (wy, wlen) in p['wheels']:
        for y in range(wy, wy + wlen):
            for dx in (0, 1):
                put(g, W // 2 - half - 1 + dx, y, 'T')
                put(g, W // 2 + half - dx, y, 'T')

    # --- lights -------------------------------------------------------------
    for dx in p['lights']:
        put(g, W // 2 - dx - 1, nose_y, 'H')
        put(g, W // 2 + dx, nose_y, 'H')
        put(g, W // 2 - dx - 1, tail_y, 'B')
        put(g, W // 2 + dx, tail_y, 'B')

    # --- optional rear wing: a bar wider than the body ----------------------
    if p.get('wing'):
        wy = tail_y + 1
        wide = half + 2
        row_span(g, wy, wide, '2', outline=False)
        row_span(g, wy - 1, wide, 'O', outline=False)

    return [''.join(r) for r in g]


def taper(top, bottom, half_mid, nose_half, tail_half):
    """Build a body profile: narrow at the nose, full width in the middle."""
    rows = []
    mid0 = top + 3
    mid1 = bottom - 3
    for y in range(top, bottom + 1):
        if y < mid0:
            f = (y - top) / max(1, mid0 - top)
            half = round(nose_half + (half_mid - nose_half) * f)
        elif y > mid1:
            f = (y - mid1) / max(1, bottom - mid1)
            half = round(half_mid + (tail_half - half_mid) * f)
        else:
            half = half_mid
        rows.append((y, half))
    return rows


CHASSIS = {
    # compact 80s coupe — narrow, upright cabin
    'ae_coupe': dict(body=taper(2, 30, 6, 4, 5), cabin=(12, 22), cabin_half=5,
                     screen=2, rear_screen=2, wheels=[(5, 5), (22, 5)], lights=[2, 4]),
    # short hatch — stubby, cabin set back
    'eg_hatch': dict(body=taper(4, 29, 6, 4, 5), cabin=(13, 23), cabin_half=5,
                     screen=2, rear_screen=1, wheels=[(7, 5), (21, 5)], lights=[2, 4]),
    # roadster — narrow, tiny cabin, long bonnet
    's_roadster': dict(body=taper(2, 30, 5, 3, 4), cabin=(16, 22), cabin_half=4,
                       screen=2, rear_screen=1, wheels=[(6, 5), (22, 5)], lights=[2]),
    # rotary — long pointed nose, low cabin
    'fc_rotary': dict(body=taper(1, 31, 6, 3, 5), cabin=(15, 24), cabin_half=5,
                      screen=3, rear_screen=2, wheels=[(6, 5), (23, 5)], lights=[2, 4]),
    # mid-engine — cabin forward, long tail deck
    'mr_mid': dict(body=taper(3, 30, 6, 4, 6), cabin=(10, 19), cabin_half=5,
                   screen=2, rear_screen=2, wheels=[(6, 5), (22, 6)], lights=[2, 4]),
    # AWD sedan — roomy cabin, rear wing
    'gc_awd': dict(body=taper(2, 29, 7, 5, 6), cabin=(11, 23), cabin_half=6,
                   screen=2, rear_screen=2, wheels=[(5, 6), (21, 6)], lights=[2, 4, 5],
                   wing=True),
    # wide GT — widest body, biggest wheels, wing
    'r_gt': dict(body=taper(1, 29, 8, 5, 7), cabin=(11, 23), cabin_half=6,
                 screen=2, rear_screen=2, wheels=[(5, 7), (20, 7)], lights=[2, 4, 6],
                 wing=True),
}


def png(path, rows, scale, bg=(11, 12, 15)):
    h, w = len(rows) * scale, len(rows[0]) * scale
    raw = b''
    for y in range(len(rows)):
        for _ in range(scale):
            line = b'\x00'
            for x in range(len(rows[y])):
                c = PALETTE.get(rows[y][x]) or bg
                line += bytes(c) * scale
            raw += line

    def chunk(tag, data):
        return (struct.pack('>I', len(data)) + tag + data +
                struct.pack('>I', zlib.crc32(tag + data) & 0xffffffff))

    with open(path, 'wb') as f:
        f.write(b'\x89PNG\r\n\x1a\n')
        f.write(chunk(b'IHDR', struct.pack('>IIBBBBB', w, h, 8, 2, 0, 0, 0)))
        f.write(chunk(b'IDAT', zlib.compress(raw, 9)))
        f.write(chunk(b'IEND', b''))


def build():
    out = {}
    for name, p in CHASSIS.items():
        rows = make_car(p)
        for i, r in enumerate(rows):
            assert len(r) == W, f"{name} row {i}: {len(r)}"
        unknown = {c for r in rows for c in r} - set(PALETTE)
        assert not unknown, f"{name}: {unknown}"
        out[name] = rows
    return out


if __name__ == '__main__':
    cars = build()
    # lay them out side by side with a gap
    gap = 3
    sheet = ['' for _ in range(H)]
    for rows in cars.values():
        for y in range(H):
            sheet[y] += rows[y] + '.' * gap
    png(f'{OUT}/topdown.png', sheet, 9)
    print(f"ok — {len(cars)} top-down cars at {W}x{H}")
