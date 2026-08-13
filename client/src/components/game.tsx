import React from 'react';
import { StyleSheet, Text, View } from 'react-native';
import { colors, radius, rarityColor, rarityLabel, space, statLabel, text as t } from '../theme';
import type { Build, CarStats, DriverStats, LiveryOption } from '../types';
import { PixelCar } from '../pixel/sprite';
import { Bar, Pill, Row } from './ui';

/**
 * A top-down car in 8-bit, tinted by the equipped livery. The chassis decides
 * the silhouette, so the ladder shows what people actually drive.
 * Sprites are portrait (nose up), so `width` stays small.
 */
export function CarArt({
  primary,
  secondary,
  width = 120,
  chassisId = 'ae_coupe',
}: {
  primary: string;
  secondary: string;
  width?: number;
  chassisId?: string;
}) {
  return <PixelCar chassisId={chassisId} primary={primary} secondary={secondary} width={width} />;
}

export function LiveryColors(livery: LiveryOption | undefined) {
  return { primary: livery?.primary ?? '#F2F2F0', secondary: livery?.secondary ?? '#16181C' };
}

export function Money({ cash, rubies }: { cash?: number; rubies?: number }) {
  return (
    <Row gap={3}>
      {cash !== undefined && (
        <Row gap={1}>
          <Text style={[st.currencyMark, { color: colors.cash }]}>¥</Text>
          <Text style={st.currencyValue}>{cash.toLocaleString()}</Text>
        </Row>
      )}
      {rubies !== undefined && (
        <Row gap={1}>
          <Text style={[st.currencyMark, { color: colors.ruby }]}>◆</Text>
          <Text style={st.currencyValue}>{rubies.toLocaleString()}</Text>
        </Row>
      )}
    </Row>
  );
}

export function RarityBadge({ rarity }: { rarity: string }) {
  const c = rarityColor[rarity] ?? colors.textDim;
  return (
    <Pill color={c} bg={c + '1A'}>
      {rarityLabel[rarity] ?? rarity}
    </Pill>
  );
}

/** Driver stats with bars scaled against the strongest stat on show. */
export function DriverStatBlock({
  stats,
  compare,
  max,
}: {
  stats: DriverStats;
  compare?: DriverStats;
  max?: number;
}) {
  const keys = Object.keys(statLabel) as (keyof DriverStats)[];
  const ceiling = max ?? Math.max(20, ...keys.map((k) => stats[k]), ...(compare ? keys.map((k) => compare[k]) : []));
  return (
    <View style={{ gap: space(2) }}>
      {keys.map((k) => {
        const mine = stats[k];
        const theirs = compare?.[k];
        const better = theirs === undefined ? undefined : mine > theirs;
        return (
          <View key={k} style={{ gap: 3 }}>
            <Row style={{ justifyContent: 'space-between' }}>
              <Text style={t.small}>{statLabel[k]}</Text>
              <Row gap={2}>
                <Text
                  style={[
                    st.statValue,
                    better === true && { color: colors.green },
                    better === false && { color: colors.red },
                  ]}>
                  {mine.toFixed(0)}
                </Text>
                {theirs !== undefined && <Text style={t.tiny}>vs {theirs.toFixed(0)}</Text>}
              </Row>
            </Row>
            <Bar value={mine} max={ceiling} color={colors.accent} height={4} />
          </View>
        );
      })}
    </View>
  );
}

export function CarStatBlock({ car }: { car: CarStats }) {
  const rows: [string, string][] = [
    ['Power', `${car.power.toFixed(0)} hp`],
    ['Weight', `${car.weight.toFixed(0)} kg`],
    ['Power / tonne', `${car.power_to_wt.toFixed(0)} hp/t`],
    ['Grip', car.grip.toFixed(0)],
    ['Handling', car.handling.toFixed(0)],
    ['Braking', car.braking.toFixed(0)],
    ['Layout', car.drivetrain],
  ];
  return (
    <View style={{ gap: space(1) }}>
      {rows.map(([k, v]) => (
        <Row key={k} style={{ justifyContent: 'space-between' }}>
          <Text style={t.small}>{k}</Text>
          <Text style={st.statValue}>{v}</Text>
        </Row>
      ))}
    </View>
  );
}

export function RatingBadge({ rating, label = 'Rating' }: { rating: number; label?: string }) {
  return (
    <View style={st.rating}>
      <Text style={t.label}>{label}</Text>
      <Text style={st.ratingValue}>{rating.toFixed(1)}</Text>
    </View>
  );
}

export function BuildSummary({ build }: { build: Build }) {
  return (
    <Row gap={4} style={{ alignItems: 'flex-start' }}>
      <View style={{ flex: 1 }}>
        <CarStatBlock car={build.car} />
      </View>
      <View style={{ flex: 1 }}>
        <DriverStatBlock stats={build.driver} />
      </View>
    </Row>
  );
}

/** Formats a countdown as m:ss or h:mm:ss. */
export function formatDuration(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`;
  return `${m}:${String(sec).padStart(2, '0')}`;
}

export const st = StyleSheet.create({
  currencyMark: { fontSize: 13, fontWeight: '700' },
  currencyValue: { fontSize: 13, fontWeight: '700', color: colors.text, fontVariant: ['tabular-nums'] },
  statValue: { fontSize: 13, fontWeight: '700', color: colors.text, fontVariant: ['tabular-nums'] },
  rating: {
    alignItems: 'flex-end',
    borderLeftWidth: 2,
    borderLeftColor: colors.accent,
    paddingLeft: space(3),
  },
  ratingValue: {
    fontSize: 26,
    fontWeight: '800',
    color: colors.accent,
    fontVariant: ['tabular-nums'],
    lineHeight: 30,
  },
  timeCell: {
    fontSize: 12,
    color: colors.text,
    fontVariant: ['tabular-nums'],
    textAlign: 'right',
    width: 52,
  },
  gapCell: {
    fontSize: 12,
    fontWeight: '700',
    fontVariant: ['tabular-nums'],
    textAlign: 'right',
    width: 60,
  },
  segRow: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingVertical: space(2),
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
    gap: space(2),
  },
  itemCard: {
    backgroundColor: colors.surfaceAlt,
    borderRadius: radius.md,
    borderWidth: 1,
    borderColor: colors.border,
    padding: space(3),
    gap: space(2),
  },
});
