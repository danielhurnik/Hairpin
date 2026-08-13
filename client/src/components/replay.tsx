import React from 'react';
import { Text, View } from 'react-native';
import { colors, space, text as t } from '../theme';
import type { SegmentResult } from '../types';
import { st } from './game';
import { Row } from './ui';

/**
 * One sector's timing line. Shared by the live race and by stored replays —
 * both render exactly the same record, because that is all a replay is.
 */
export function SegmentRow({ seg }: { seg: SegmentResult }) {
  const won = seg.challenger_time < seg.opponent_time;
  return (
    <View style={st.segRow}>
      <Text style={[st.timeCell, { color: won ? colors.green : colors.textDim }]}>
        {seg.challenger_time.toFixed(2)}
      </Text>
      <View style={{ flex: 1 }}>
        <Text style={[t.small, { color: colors.text }]}>{seg.name}</Text>
        {(seg.challenger_mistake || seg.opponent_mistake) && (
          <Text style={[t.tiny, { color: colors.red }]}>
            {seg.challenger_mistake && seg.opponent_mistake
              ? 'both ran wide'
              : seg.challenger_mistake
                ? 'you ran wide'
                : 'rival ran wide'}
          </Text>
        )}
      </View>
      <Text style={[st.timeCell, { color: !won ? colors.green : colors.textDim }]}>
        {seg.opponent_time.toFixed(2)}
      </Text>
      <Text style={[st.gapCell, { color: seg.gap >= 0 ? colors.green : colors.red }]}>
        {seg.gap >= 0 ? '+' : ''}
        {seg.gap.toFixed(2)}
      </Text>
    </View>
  );
}

export function SectorTable({ segments }: { segments: SegmentResult[] }) {
  return (
    <View>
      <Row style={{ paddingBottom: space(1), gap: space(2) }}>
        <Text style={[t.label, { width: 52, textAlign: 'right' }]}>You</Text>
        <Text style={[t.label, { flex: 1 }]}>Sector</Text>
        <Text style={[t.label, { width: 52, textAlign: 'right' }]}>Rival</Text>
        <Text style={[t.label, { width: 60, textAlign: 'right' }]}>Gap</Text>
      </Row>
      {segments.map((seg) => (
        <SegmentRow key={seg.index} seg={seg} />
      ))}
    </View>
  );
}

/** A centre-zero bar: fills right when you lead, left when you trail. */
export function GapMeter({ gap }: { gap: number }) {
  const clamped = Math.max(-3, Math.min(3, gap));
  const pct = (Math.abs(clamped) / 3) * 100;
  const ahead = gap >= 0;
  return (
    <View style={{ gap: space(1) }}>
      <View
        style={{
          height: 10,
          backgroundColor: colors.raised,
          borderRadius: 5,
          flexDirection: 'row',
          overflow: 'hidden',
        }}>
        <View style={{ flex: 1, flexDirection: 'row', justifyContent: 'flex-end' }}>
          {!ahead && <View style={{ width: `${pct}%`, height: '100%', backgroundColor: colors.red }} />}
        </View>
        <View style={{ width: 2, backgroundColor: colors.borderBright }} />
        <View style={{ flex: 1, flexDirection: 'row' }}>
          {ahead && <View style={{ width: `${pct}%`, height: '100%', backgroundColor: colors.green }} />}
        </View>
      </View>
      <Text
        style={{
          alignSelf: 'center',
          fontSize: 16,
          fontWeight: '700',
          fontVariant: ['tabular-nums'],
          color: ahead ? colors.green : colors.red,
        }}>
        {ahead ? '+' : ''}
        {gap.toFixed(2)}
      </Text>
    </View>
  );
}
