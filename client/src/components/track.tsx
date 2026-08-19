import React, { useState } from 'react';
import { StyleSheet, Text, View, type LayoutChangeEvent } from 'react-native';
import Svg, { Line, Path, Rect } from 'react-native-svg';
import { colors, font, space, text as t } from '../theme';
import type { TrackDef, TrackSegment } from '../types';
import { Pill, Row } from './ui';

const surfaceLabel: Record<string, string> = {
  smooth: 'Smooth',
  coarse: 'Coarse',
  patched: 'Patched',
  broken: 'Broken',
};

/** Tight corners are the interesting ones, so the scale is on radius, not length. */
function tightness(seg: TrackSegment): number {
  const r = seg.radius_m ?? 0;
  if (r <= 0) return 0;
  return Math.max(0, Math.min(1, (140 - r) / 125));
}

function cornerColor(seg: TrackSegment): string {
  const hot = tightness(seg);
  if (hot > 0.75) return colors.red;
  if (hot > 0.45) return colors.accent;
  return colors.blue;
}

/**
 * The road, drawn from the same geometry the race is resolved with.
 *
 * Two strips: what the road does vertically, and what it does horizontally.
 * The height trace is cumulative gradient over distance, so a descent visibly
 * falls away; the ribbon underneath puts every corner where it happens along
 * the pass, left of the centre line for a left and right for a right, coloured
 * by how tight it is. Between them they answer "what am I setting the car up
 * for" without a single number having to be read.
 */
export function RoadProfile({ track }: { track: TrackDef }) {
  const [width, setWidth] = useState(0);
  const H = 108;
  const profileH = 62;
  const ribbonTop = profileH + 12;
  const ribbonH = 30;
  const mid = ribbonTop + ribbonH / 2;

  const total = track.segments.reduce((sum, s) => sum + s.length_m, 0) || 1;

  // Height, in metres, at the end of each segment.
  const heights: number[] = [];
  let h = 0;
  for (const seg of track.segments) {
    h += (seg.gradient_pct / 100) * seg.length_m;
    heights.push(h);
  }
  const lo = Math.min(0, ...heights);
  const hi = Math.max(0, ...heights);
  const range = hi - lo || 1;

  const x = (metres: number) => (metres / total) * width;
  const y = (height: number) => profileH - ((height - lo) / range) * (profileH - 6) - 3;

  let travelled = 0;
  const points: string[] = [`${x(0).toFixed(1)},${y(0).toFixed(1)}`];
  for (let i = 0; i < track.segments.length; i++) {
    travelled += track.segments[i].length_m;
    points.push(`${x(travelled).toFixed(1)},${y(heights[i]).toFixed(1)}`);
  }
  const line = `M${points.join('L')}`;
  const area = `${line}L${x(total).toFixed(1)},${profileH}L0,${profileH}Z`;

  return (
    <View onLayout={(e: LayoutChangeEvent) => setWidth(e.nativeEvent.layout.width)}>
      {width > 0 && (
        <Svg width={width} height={H}>
          <Path d={area} fill={colors.accent} fillOpacity={0.08} />
          <Path d={line} stroke={colors.accent} strokeWidth={2} fill="none" />

          {/* The road itself, seen from above and flattened out. */}
          <Line x1={0} y1={mid} x2={width} y2={mid} stroke={colors.border} strokeWidth={1} />
          {track.segments.map((seg, i) => {
            const start = track.segments.slice(0, i).reduce((sum, s) => sum + s.length_m, 0);
            const w = Math.max(1.5, x(seg.length_m));
            if (!seg.radius_m) {
              return (
                <Rect
                  key={i}
                  x={x(start)}
                  y={mid - 2}
                  width={w}
                  height={4}
                  fill={colors.borderBright}
                />
              );
            }
            const hot = tightness(seg);
            const bar = 4 + hot * 11;
            const left = seg.direction < 0;
            return (
              <Rect
                key={i}
                x={x(start)}
                y={left ? mid - bar : mid}
                width={w}
                height={bar}
                rx={1}
                fill={cornerColor(seg)}
                fillOpacity={0.85}
              />
            );
          })}
        </Svg>
      )}
      <Row style={{ justifyContent: 'space-between' }}>
        <Text style={t.tiny}>start</Text>
        <Text style={t.tiny}>{Math.round(total).toLocaleString()} m</Text>
      </Row>
    </View>
  );
}

/** The headline numbers a driver would ask about a road before driving it. */
export function TrackFacts({ track }: { track: TrackDef }) {
  const corners = track.segments.filter((s) => (s.radius_m ?? 0) > 0);
  const length = track.segments.reduce((sum, s) => sum + s.length_m, 0);
  const climbed = track.segments.reduce(
    (sum, s) => (s.gradient_pct > 0 ? sum + (s.gradient_pct / 100) * s.length_m : sum),
    0
  );
  const dropped = track.segments.reduce(
    (sum, s) => (s.gradient_pct < 0 ? sum - (s.gradient_pct / 100) * s.length_m : sum),
    0
  );
  const tightest = corners.reduce((min, s) => Math.min(min, s.radius_m ?? 999), 999);
  const narrowest = track.segments.reduce((min, s) => Math.min(min, s.width_m), 99);

  return (
    <View style={s.facts}>
      <Fact label="Length" value={`${(length / 1000).toFixed(2)} km`} />
      <Fact label="Corners" value={`${corners.length}`} />
      <Fact label="Tightest" value={corners.length ? `R${Math.round(tightest)}` : '—'} />
      <Fact label={dropped >= climbed ? 'Descent' : 'Climb'} value={`${Math.round(Math.max(dropped, climbed))} m`} />
      <Fact label="Narrowest" value={`${narrowest.toFixed(1)} m`} />
    </View>
  );
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <View style={s.fact}>
      <Text style={t.label}>{label}</Text>
      <Text style={s.factValue}>{value}</Text>
    </View>
  );
}

/** Where the geometry came from. Two different claims, so they look different. */
export function SourceBadge({ track }: { track: TrackDef }) {
  if (track.source === 'surveyed') {
    return (
      <Pill color={colors.green} bg={colors.green + '18'}>
        {track.gradient_measured ? 'SURVEYED' : 'SURVEYED · GRADIENT BORROWED'}
      </Pill>
    );
  }
  return <Pill color={colors.textFaint}>AUTHORED STAND-IN</Pill>;
}

/** Corner by corner, in the order they arrive. */
export function SegmentList({ segments }: { segments: TrackSegment[] }) {
  return (
    <View style={{ gap: space(2) }}>
      {segments.map((seg, i) => (
        <View key={i} style={s.segment}>
          <View style={[s.marker, { backgroundColor: seg.radius_m ? cornerColor(seg) : colors.raised }]} />
          <View style={{ flex: 1, gap: 2 }}>
            <Row style={{ justifyContent: 'space-between' }}>
              <Text style={s.segName}>{seg.name}</Text>
              <Text style={s.segKind}>{seg.kind}</Text>
            </Row>
            <Text style={t.tiny}>
              {seg.radius_m ? `R${Math.round(seg.radius_m)} · ` : ''}
              {Math.round(seg.length_m)}m · {seg.width_m.toFixed(1)}m wide ·{' '}
              {seg.gradient_pct > 0 ? '+' : ''}
              {seg.gradient_pct.toFixed(1)}% · {surfaceLabel[seg.surface] ?? seg.surface}
              {seg.banking_deg < 0 ? ' · off-camber' : ''}
              {seg.blind ? ' · blind' : ''}
              {seg.crest ? ' · crest' : ''}
            </Text>
          </View>
        </View>
      ))}
    </View>
  );
}

const s = StyleSheet.create({
  facts: { flexDirection: 'row', flexWrap: 'wrap', gap: space(4) },
  fact: { minWidth: 62 },
  factValue: { color: colors.text, fontFamily: font.mono, fontSize: 15, fontWeight: '700' },
  segment: { flexDirection: 'row', alignItems: 'flex-start', gap: space(3) },
  marker: { width: 4, alignSelf: 'stretch', borderRadius: 2, minHeight: 26 },
  segName: { color: colors.text, fontSize: 13, fontWeight: '600' },
  segKind: {
    color: colors.textFaint,
    fontSize: 10,
    letterSpacing: 0.8,
    textTransform: 'uppercase',
  },
});
