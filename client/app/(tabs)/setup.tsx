import React, { useState } from 'react';
import { Text, View } from 'react-native';
import { ApiError } from '../../src/api';
import { Dial } from '../../src/components/dial';
import { RoadProfile, SegmentList, SourceBadge, TrackFacts } from '../../src/components/track';
import { Banner, Bar, Button, Card, Pill, Row, SectionLabel, Screen } from '../../src/components/ui';
import { useGame } from '../../src/state';
import { colors, space, text as t } from '../../src/theme';
import type { Conditions, SetupAxis, TrackDef } from '../../src/types';

/**
 * The tuning screen.
 *
 * It shows the road, the weather and the sheet, and deliberately does not show
 * what any of it is worth. A per-track score would respond to every adjustment,
 * which turns the screen into a solver: whoever optimises against it once has
 * optimised it for everybody. What the sheet was worth arrives after the race,
 * in the notes on the result.
 */
export default function SetupScreen() {
  const { state, act, refresh } = useGame();
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [showCorners, setShowCorners] = useState(false);

  if (!state) return null;
  const sheet = state.setup;

  const run = async (key: string, fn: () => Promise<unknown>) => {
    setBusy(key);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'That did not go through');
      await refresh().catch(() => {});
    } finally {
      setBusy(null);
    }
  };

  const setAxis = (axis: SetupAxis, setting: number) =>
    run(axis.id, () => act('/v1/setup', { axis: axis.id, setting }));

  const reset = () => run('reset', () => act('/v1/setup/reset'));

  return (
    <Screen onRefresh={refresh}>
      {error && <Banner tone="error">{error}</Banner>}

      <RoadCard track={sheet.track} />
      <ConditionsCard conditions={sheet.conditions} />

      <SectionLabel right={<Text style={t.tiny}>{sheet.chassis_name}</Text>}>Setup sheet</SectionLabel>

      {!sheet.any_unlocked && (
        <Banner tone="info">
          A stock car has nothing to turn. Coilovers, arms, dampers, a boost controller — each
          upgrade tier in the garage hands over another spanner.
        </Banner>
      )}

      {sheet.axes.map((axis) => (
        <AxisCard
          key={axis.id}
          axis={axis}
          busy={busy === axis.id}
          onSet={(setting) => setAxis(axis, setting)}
        />
      ))}

      {sheet.any_unlocked && (
        <>
          <Button
            title="Back to factory"
            variant="ghost"
            busy={busy === 'reset'}
            onPress={reset}
          />
          <Text style={t.tiny}>
            Adjusting is free and unlimited — the money went on the parts. A setup moves
            performance from the parts of the road you have decided not to care about onto the
            parts you have; it never creates any, which is why your rating does not move when you
            turn something.
          </Text>
        </>
      )}

      <Button
        title={showCorners ? 'Hide the corners' : 'Corner by corner'}
        variant="ghost"
        onPress={() => setShowCorners((v) => !v)}
      />
      {showCorners && (
        <Card>
          <SegmentList segments={sheet.track.segments} />
        </Card>
      )}
    </Screen>
  );
}

function RoadCard({ track }: { track: TrackDef }) {
  return (
    <Card>
      <Row style={{ justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <View style={{ flex: 1 }}>
          <Text style={t.label}>Tonight's pass</Text>
          <Text style={t.title}>{track.name}</Text>
        </View>
        <SourceBadge track={track} />
      </Row>

      <Text style={[t.small, { fontStyle: 'italic' }]}>{track.flavour}</Text>

      <RoadProfile track={track} />
      <TrackFacts track={track} />

      {track.real_world ? <Text style={t.tiny}>{track.real_world}</Text> : null}
      {track.source === 'authored' && (
        <Text style={t.tiny}>
          The geometry above is written to match what this road is known for, and is not a survey
          of it. Every corner in it is real to the race — the radii are simply someone's judgement
          rather than a measurement.
        </Text>
      )}
    </Card>
  );
}

function ConditionsCard({ conditions }: { conditions: Conditions }) {
  const wet = conditions.damp > 0.45;
  const cold = conditions.temp_c < 10;
  return (
    <Card tone={wet ? 'accent' : 'default'}>
      <Row style={{ justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <View style={{ flex: 1 }}>
          <Text style={t.label}>Conditions</Text>
          <Text style={t.heading}>{conditions.label}</Text>
        </View>
        <Row gap={2}>
          {cold && <Pill color={colors.blue}>COLD</Pill>}
          {wet && <Pill color={colors.blue}>WET</Pill>}
        </Row>
      </Row>

      <Text style={t.small}>{conditions.note}</Text>

      <View style={{ gap: space(2) }}>
        <Row style={{ justifyContent: 'space-between' }}>
          <Text style={t.tiny}>Water on the road</Text>
          <Text style={t.tiny}>{Math.round(conditions.damp * 100)}%</Text>
        </Row>
        <Bar value={conditions.damp} max={1} color={colors.blue} />
        <Row style={{ justifyContent: 'space-between' }}>
          <Text style={t.tiny}>Surface temperature</Text>
          <Text style={t.tiny}>{conditions.temp_c.toFixed(0)}°C</Text>
        </Row>
        <Bar value={conditions.temp_c} max={30} color={conditions.temp_c < 10 ? colors.blue : colors.accent} />
      </View>

      <Text style={t.tiny}>
        Fixed before you tune and revealed with the road, so it is a decision rather than a gamble —
        just never the same decision twice.
      </Text>
    </Card>
  );
}

function AxisCard({
  axis,
  busy,
  onSet,
}: {
  axis: SetupAxis;
  busy: boolean;
  onSet: (setting: number) => void;
}) {
  if (!axis.unlocked) {
    return (
      <Card>
        <Row style={{ justifyContent: 'space-between', alignItems: 'flex-start' }}>
          <View style={{ flex: 1 }}>
            <Text style={[t.heading, { color: colors.textFaint }]}>{axis.label}</Text>
            <Text style={t.tiny}>{axis.requires}</Text>
          </View>
          <Pill color={colors.textFaint}>LOCKED</Pill>
        </Row>
      </Card>
    );
  }

  const moved = Math.abs(axis.setting - axis.factory) > axis.step / 2;

  return (
    <Card tone={moved ? 'accent' : 'default'}>
      <Dial
        label={axis.label}
        sublabel={axis.part}
        value={axis.setting}
        low={axis.setting_low}
        high={axis.setting_high}
        step={axis.step}
        decimals={axis.decimals}
        unit={axis.unit}
        lowLabel={axis.low}
        highLabel={axis.high}
        factory={axis.factory}
        busy={busy}
        onCommit={onSet}
      />
      <Text style={t.small}>{axis.note}</Text>
    </Card>
  );
}
