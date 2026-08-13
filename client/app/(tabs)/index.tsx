import React, { useState } from 'react';
import { Text, View } from 'react-native';
import { ApiError } from '../../src/api';
import { CarArt, CarStatBlock, LiveryColors, Money, RatingBadge } from '../../src/components/game';
import { Banner, Button, Card, Row, SectionLabel, Screen } from '../../src/components/ui';
import { useGame } from '../../src/state';
import { colors, space, text as t } from '../../src/theme';
import type { UpgradeOption } from '../../src/types';

export default function GarageScreen() {
  const { state, act, refresh } = useGame();
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  if (!state) return null;
  const livery = state.liveries.find((l) => l.id === state.player.livery_id);
  const { primary, secondary } = LiveryColors(livery);

  const buy = async (cat: string) => {
    setBusy(cat);
    setError(null);
    try {
      await act('/v1/garage/upgrade', { category: cat });
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Purchase failed');
    } finally {
      setBusy(null);
    }
  };

  return (
    <Screen onRefresh={refresh}>
      {error && <Banner tone="error">{error}</Banner>}

      <Card>
        <Row style={{ justifyContent: 'space-between', alignItems: 'flex-start' }}>
          <View style={{ flex: 1 }}>
            <Text style={t.label}>Your car</Text>
            <Text style={t.title}>{chassisName(state.player.chassis_id)}</Text>
            <Text style={t.small}>{livery?.name ?? 'Unpainted'}</Text>
          </View>
          <RatingBadge rating={state.rating} />
        </Row>

        <View style={{ alignItems: 'center', paddingVertical: space(2) }}>
          <CarArt primary={primary} secondary={secondary} chassisId={state.player.chassis_id} />
        </View>

        <CarStatBlock car={state.build.car} />
      </Card>

      <SectionLabel right={<Money cash={state.player.cash} />}>Upgrades</SectionLabel>

      {state.garage.map((u) => (
        <UpgradeRow
          key={u.category}
          upgrade={u}
          busy={busy === u.category}
          onBuy={() => buy(u.category)}
        />
      ))}

      <Text style={[t.tiny, { marginTop: space(2) }]}>
        Rating deltas are computed by the server using the same model that resolves races.
      </Text>
    </Screen>
  );
}

function UpgradeRow({
  upgrade,
  busy,
  onBuy,
}: {
  upgrade: UpgradeOption;
  busy: boolean;
  onBuy: () => void;
}) {
  const pips = Array.from({ length: upgrade.max_tier }, (_, i) => i < upgrade.tier);
  return (
    <Card>
      <Row style={{ justifyContent: 'space-between' }}>
        <View style={{ flex: 1 }}>
          <Text style={t.heading}>{upgrade.label}</Text>
          <Text style={t.small}>{upgrade.tier_name}</Text>
        </View>
        <Row gap={1}>
          {pips.map((filled, i) => (
            <View
              key={i}
              style={{
                width: 14,
                height: 6,
                borderRadius: 2,
                backgroundColor: filled ? colors.accent : colors.raised,
              }}
            />
          ))}
        </Row>
      </Row>

      {upgrade.maxed ? (
        <Banner tone="good">Fully built. Nothing left to bolt on.</Banner>
      ) : (
        <>
          <Row style={{ justifyContent: 'space-between' }}>
            <Text style={t.small}>
              Next: <Text style={{ color: colors.text }}>{upgrade.next_tier_name}</Text>
            </Text>
            {upgrade.rating_delta ? (
              <Text style={[t.small, { color: colors.green }]}>
                +{upgrade.rating_delta.toFixed(2)} rating
              </Text>
            ) : null}
          </Row>
          <Button
            title={`¥${(upgrade.next_cost ?? 0).toLocaleString()}`}
            subtitle={upgrade.affordable ? undefined : 'Not enough cash'}
            variant={upgrade.affordable ? 'primary' : 'default'}
            disabled={!upgrade.affordable}
            busy={busy}
            onPress={onBuy}
          />
        </>
      )}
    </Card>
  );
}

const CHASSIS_NAMES: Record<string, string> = {
  ae_coupe: 'AE-Series Coupe',
  eg_hatch: 'EG-Series Hatch',
  s_roadster: 'S-Series Roadster',
  fc_rotary: 'FC-Series Rotary',
  mr_mid: 'MR-Series Mid-Engine',
  gc_awd: 'GC-Series AWD',
  r_gt: 'R-Series GT',
};

function chassisName(id: string) {
  return CHASSIS_NAMES[id] ?? id;
}
