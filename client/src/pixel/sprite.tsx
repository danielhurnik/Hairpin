import React, { useMemo } from 'react';
import Svg, { Rect } from 'react-native-svg';
import { colors } from '../theme';
import { CAR_H, CAR_SPRITES, CAR_W, ICON_SPRITES } from './sprites';

/** char -> colour. A missing or undefined entry means transparent. */
export type Palette = Record<string, string | undefined>;

type Run = { x: number; y: number; w: number; fill: string };

/**
 * Collapses each row into horizontal runs of one colour. A car sprite is
 * 48x20 = 960 cells but only ~60 runs, which keeps the SVG cheap enough to
 * render a whole ladder page of them.
 */
function runsFor(rows: string[], palette: Palette): Run[] {
  const out: Run[] = [];
  for (let y = 0; y < rows.length; y++) {
    const row = rows[y];
    let x = 0;
    while (x < row.length) {
      const ch = row[x];
      let end = x;
      while (end < row.length && row[end] === ch) end++;
      const fill = palette[ch];
      if (fill) out.push({ x, y, w: end - x, fill });
      x = end;
    }
  }
  return out;
}

export function PixelSprite({
  rows,
  palette,
  width,
  spriteWidth,
  spriteHeight,
}: {
  rows: string[];
  palette: Palette;
  width: number;
  spriteWidth: number;
  spriteHeight: number;
}) {
  const runs = useMemo(() => runsFor(rows, palette), [rows, palette]);
  const height = (width * spriteHeight) / spriteWidth;
  return (
    <Svg width={width} height={height} viewBox={`0 0 ${spriteWidth} ${spriteHeight}`}>
      {runs.map((r, i) => (
        <Rect key={i} x={r.x} y={r.y} width={r.w} height={1} fill={r.fill} />
      ))}
    </Svg>
  );
}

// ---------------------------------------------------------------------------
// Cars
// ---------------------------------------------------------------------------

/** Mixes a hex colour toward black by `amount` (0..1). */
function shade(hex: string, amount: number): string {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex.trim());
  if (!m) return hex;
  const n = parseInt(m[1], 16);
  const mix = (c: number) => Math.max(0, Math.min(255, Math.round(c * (1 - amount))));
  const r = mix((n >> 16) & 0xff);
  const g = mix((n >> 8) & 0xff);
  const b = mix(n & 0xff);
  return `#${((r << 16) | (g << 8) | b).toString(16).padStart(6, '0')}`;
}

/**
 * The livery drives only the body and trim colours — glass, tyres and lights
 * are fixed, so every car stays readable whatever paint is on it.
 */
export function carPalette(primary: string, secondary: string): Palette {
  return {
    '.': undefined,
    O: '#4A5260',
    '1': primary,
    '2': secondary,
    '3': shade(primary, 0.35),
    G: '#485C76',
    T: '#282A30',
    R: '#A8B0BE',
    H: colors.accent,
    B: colors.red,
  };
}

export function PixelCar({
  chassisId,
  primary,
  secondary,
  width = 220,
}: {
  chassisId: string;
  primary: string;
  secondary: string;
  width?: number;
}) {
  const rows = CAR_SPRITES[chassisId] ?? CAR_SPRITES.ae_coupe;
  const palette = useMemo(() => carPalette(primary, secondary), [primary, secondary]);
  return (
    <PixelSprite
      rows={rows}
      palette={palette}
      width={width}
      spriteWidth={CAR_W}
      spriteHeight={CAR_H}
    />
  );
}

// ---------------------------------------------------------------------------
// Icons — 1-bit masks tinted at the call site
// ---------------------------------------------------------------------------

export function PixelIcon({
  name,
  color,
  size = 22,
}: {
  name: keyof typeof ICON_SPRITES | string;
  color: string;
  size?: number;
}) {
  const rows = ICON_SPRITES[name];
  const palette = useMemo<Palette>(() => ({ '.': undefined, '#': color }), [color]);
  if (!rows) return null;
  return (
    <PixelSprite
      rows={rows}
      palette={palette}
      width={size}
      spriteWidth={rows[0].length}
      spriteHeight={rows.length}
    />
  );
}
