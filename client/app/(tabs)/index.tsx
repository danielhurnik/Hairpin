import React, { useState } from 'react';
import { Text, View } from 'react-native';
import { ApiError } from '../../src/api';
import { CarArt, CarStatBlock, LiveryColors, Money, RatingBadge } from '../../src/components/game';
import { Banner, Button, Card, Pill, Row, SectionLabel, Screen } from '../../src/components/ui';
import { useGame } from '../../src/state';
import { colors, space, text as t } from '../../src/theme';
import type { CarOption, UpgradeOption } from '../../src/types';

export default function GarageScreen() {
  const { state, act, refresh } = useGame();
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  if (!state) return null;
  const livery = state.liveries.find((l) => l.id === state.player.livery_id);
  const { primary, secondary } = LiveryColors(livery);

  const active = state.cars.find((c) => c.active);
  const garage = state.cars.filter((c) => c.owned && !c.active);
  const showroom = state.cars.filter((c) => !c.owned);

  const run = async (key: string, fn: () => Promise<unknown>) => {
    setBusy(key);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'That did not go through');
    } finally {
      setBusy(null);
    }
  };

  const buyUpgrade = (cat: string) => run(cat, () => act('/v1/garage/upgrade', { category: cat }));
  const buyCar = (id: string) => run(`buy:${id}`, () => act('/v1/cars/buy', { chassis_id: id }));
  const driveCar = (id: string) => run(`drive:${id}`, () => act('/v1/cars/equip', { chassis_id: id }));

  return (
    <Screen onRefresh={refresh}>
      {error && <Banner tone="error">{error}</Banner>}

      <Card>
        <Row style={{ justifyContent: 'space-between', alignItems: 'flex-start' }}>
          <View style={{ flex: 1 }}>
            <Text style={t.label}>Your car</Text>
            <Text style={t.title}>{active?.name ?? state.player.chassis_id}</Text>
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
          onBuy={() => buyUpgrade(u.category)}
        />
      ))}

      <Text style={[t.tiny, { marginTop: space(2) }]}>
        Rating deltas are computed by the server using the same model that resolves races.
      </Text>

      {garage.length > 0 && (
        <>
          <SectionLabel>Also in the garage</SectionLabel>
          {garage.map((car) => (
            <OwnedCarRow
              key={car.id}
              car={car}
              primary={primary}
              secondary={secondary}
              busy={busy === `drive:${car.id}`}
              onDrive={() => driveCar(car.id)}
            />
          ))}
          <Text style={t.tiny}>
            Each car keeps its own upgrades. Switching is free — the build you paid for is still
            bolted to the car you left behind.
          </Text>
        </>
      )}

      {showroom.length > 0 && (
        <>
          <SectionLabel right={<Money cash={state.player.cash} />}>Showroom</SectionLabel>
          {showroom.map((car) => (
            <ShowroomRow
              key={car.id}
              car={car}
              level={state.player.driver_level}
              primary={primary}
              secondary={secondary}
              busy={busy === `buy:${car.id}`}
              onBuy={() => buyCar(car.id)}
            />
          ))}
          <Text style={t.tiny}>
            A new car arrives stock. The rating shown is what it would race at today — before you
            spend anything building it.
          </Text>
        </>
      )}
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

/** How built a car is, at a glance, without listing five categories. */
function BuildPips({ car }: { car: CarOption }) {
  const tiers = [
    car.upgrades.engine,
    car.upgrades.turbo,
    car.upgrades.tires,
    car.upgrades.suspension,
    car.upgrades.weight,
  ];
  return (
    <Row gap={1}>
      {tiers.map((tier, i) => (
        <View
          key={i}
          style={{
            width: 10,
            height: 6,
            borderRadius: 2,
            backgroundColor: tier > 0 ? colors.accent : colors.raised,
            opacity: tier > 0 ? 0.4 + (tier / 5) * 0.6 : 1,
          }}
        />
      ))}
    </Row>
  );
}

function OwnedCarRow({
  car,
  primary,
  secondary,
  busy,
  onDrive,
}: {
  car: CarOption;
  primary: string;
  secondary: string;
  busy: boolean;
  onDrive: () => void;
}) {
  return (
    <Card>
      <Row gap={3} style={{ alignItems: 'flex-start' }}>
        <CarArt primary={primary} secondary={secondary} chassisId={car.id} width={72} />
        <View style={{ flex: 1, gap: space(1) }}>
          <Text style={t.heading}>{car.name}</Text>
          <Text style={t.tiny}>
            {car.drivetrain} · {car.power.toFixed(0)} hp · {car.weight.toFixed(0)} kg
          </Text>
          <BuildPips car={car} />
        </View>
        <RatingBadge rating={car.rating} />
      </Row>
      <Button
        title="Drive this one"
        subtitle={deltaLabel(car.rating_delta)}
        variant="ghost"
        busy={busy}
        onPress={onDrive}
      />
    </Card>
  );
}

function ShowroomRow({
  car,
  level,
  primary,
  secondary,
  busy,
  onBuy,
}: {
  car: CarOption;
  level: number;
  primary: string;
  secondary: string;
  busy: boolean;
  onBuy: () => void;
}) {
  const locked = !car.level_met;
  const canBuy = car.level_met && car.affordable;

  return (
    <Card tone={locked ? 'default' : 'accent'}>
      <Row gap={3} style={{ alignItems: 'flex-start' }}>
        <View style={{ opacity: locked ? 0.35 : 1 }}>
          <CarArt primary={primary} secondary={secondary} chassisId={car.id} width={72} />
        </View>
        <View style={{ flex: 1, gap: space(1) }}>
          <Text style={t.heading}>{car.name}</Text>
          <Text style={t.tiny}>
            {car.drivetrain} · {car.power.toFixed(0)} hp · {car.weight.toFixed(0)} kg
          </Text>
          {locked ? (
            <Pill color={colors.textFaint}>LEVEL {car.min_level}</Pill>
          ) : (
            <Pill color={colors.accent}>STOCK · RATING {car.rating.toFixed(1)}</Pill>
          )}
        </View>
      </Row>

      <Text style={t.small}>{car.blurb}</Text>

      {locked ? (
        <Button
          title={`¥${car.price.toLocaleString()}`}
          subtitle={`Driver level ${car.min_level} required — you are ${level}`}
          disabled
          onPress={onBuy}
        />
      ) : (
        <>
          {car.rating_delta < 0 && (
            <Banner tone="info">
              Stock, this is {Math.abs(car.rating_delta).toFixed(1)} rating slower than what you
              drive now. Upgrades do not come with it.
            </Banner>
          )}
          <Button
            title={`¥${car.price.toLocaleString()}`}
            subtitle={car.affordable ? deltaLabel(car.rating_delta) : 'Not enough cash'}
            variant={canBuy ? 'primary' : 'default'}
            disabled={!canBuy}
            busy={busy}
            onPress={onBuy}
          />
        </>
      )}
    </Card>
  );
}

/** Reads the same either way: the sign is the point. */
function deltaLabel(delta: number) {
  if (!delta) return undefined;
  return `${delta > 0 ? '+' : ''}${delta.toFixed(1)} rating`;
}
