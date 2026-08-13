import React, { useCallback, useEffect, useState } from 'react';
import { Text, View } from 'react-native';
import { ApiError, api } from '../../src/api';
import { CarArt, LiveryColors, Money, RarityBadge } from '../../src/components/game';
import { Banner, Button, Card, Loading, Row, Screen, SectionLabel } from '../../src/components/ui';
import { useGame } from '../../src/state';
import { colors, rarityColor, slotLabel, space, text as t } from '../../src/theme';
import type { LiveryOption, ShopOffer, ShopView } from '../../src/types';

export default function ShopScreen() {
  const { state, act, refresh } = useGame();
  const [shop, setShop] = useState<ShopView | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);

  const load = useCallback(async () => {
    setShop(await api.get<ShopView>('/v1/shop'));
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  if (!state) return null;

  const run = async (key: string, fn: () => Promise<unknown>) => {
    setBusy(key);
    setError(null);
    setNote(null);
    try {
      await fn();
      await Promise.all([load(), refresh()]);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Purchase failed');
    } finally {
      setBusy(null);
    }
  };

  return (
    <Screen
      onRefresh={async () => {
        await Promise.all([refresh(), load()]);
      }}>
      {error && <Banner tone="error">{error}</Banner>}
      {note && <Banner tone="good">{note}</Banner>}

      <Card>
        <Text style={t.label}>K's Office</Text>
        <Text style={t.title}>Back room, second floor</Text>
        <Text style={[t.small, { fontStyle: 'italic' }]}>
          "Everything here dropped for somebody. You're just paying not to wait."
        </Text>
        <Row style={{ justifyContent: 'space-between' }}>
          <Text style={t.tiny}>Stock rotates daily · {shop?.day_key ?? '—'}</Text>
          <Money cash={state.player.cash} rubies={state.player.rubies} />
        </Row>
      </Card>

      <SectionLabel>Today's stock</SectionLabel>
      {!shop ? (
        <Loading />
      ) : (
        shop.offers.map((offer) => (
          <OfferCard
            key={offer.offer_id}
            offer={offer}
            cash={state.player.cash}
            rubies={state.player.rubies}
            busy={busy === offer.offer_id}
            onBuy={() =>
              run(offer.offer_id, async () => {
                await api.post('/v1/shop/buy', { offer_id: offer.offer_id });
                setNote(`${offer.name} is in your inventory — equip it on the Driver tab.`);
              })
            }
          />
        ))
      )}

      <SectionLabel>Liveries</SectionLabel>
      <Text style={t.tiny}>Paint only. Liveries never change a single stat.</Text>
      {state.liveries.map((l) => (
        <LiveryCard
          key={l.id}
          livery={l}
          cash={state.player.cash}
          rubies={state.player.rubies}
          busy={busy === l.id}
          chassisId={state.player.chassis_id}
          onBuy={() => run(l.id, () => act('/v1/liveries/buy', { livery_id: l.id }))}
          onEquip={() => run(l.id, () => act('/v1/liveries/equip', { livery_id: l.id }))}
        />
      ))}

      <SectionLabel>Rubies</SectionLabel>
      <Banner tone="info">
        Development build: purchases are granted without a store receipt. Real receipt validation is
        required before release.
      </Banner>
      {shop?.ruby_packs.map((pack) => (
        <Card key={pack.id}>
          <Row style={{ justifyContent: 'space-between' }}>
            <View>
              <Text style={t.heading}>{pack.name}</Text>
              <Text style={t.small}>
                ◆{pack.rubies.toLocaleString()}
                {pack.bonus > 0 ? ` + ${pack.bonus} bonus` : ''}
              </Text>
            </View>
            <Text style={[t.body, { fontWeight: '700' }]}>${pack.price_usd}</Text>
          </Row>
          <Button
            title="Purchase"
            busy={busy === pack.id}
            onPress={() =>
              run(pack.id, async () => {
                await api.post('/v1/store/purchase', {
                  pack_id: pack.id,
                  // A real build passes the signed store receipt here.
                  receipt: `dev-${pack.id}-${Date.now()}`,
                });
                setNote(`${pack.name} credited.`);
              })
            }
          />
        </Card>
      ))}
    </Screen>
  );
}

function OfferCard({
  offer,
  cash,
  rubies,
  busy,
  onBuy,
}: {
  offer: ShopOffer;
  cash: number;
  rubies: number;
  busy: boolean;
  onBuy: () => void;
}) {
  const usesRubies = !!offer.price_rubies;
  const price = usesRubies ? offer.price_rubies! : offer.price_cash!;
  const affordable = usesRubies ? rubies >= price : cash >= price;

  return (
    <Card style={offer.sold_out ? { opacity: 0.5 } : undefined}>
      <Row style={{ justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <View style={{ flex: 1, paddingRight: space(2) }}>
          <Text style={[t.heading, { color: rarityColor[offer.rarity] }]}>{offer.name}</Text>
          <Text style={t.tiny}>
            {slotLabel[offer.slot] ?? offer.slot} · quality {offer.quality.toFixed(2)}
          </Text>
        </View>
        <RarityBadge rarity={offer.rarity} />
      </Row>
      <Text style={[t.small, { fontStyle: 'italic' }]}>{offer.flavour}</Text>

      {offer.sold_out ? (
        <Banner tone="info">Sold — one of each per day.</Banner>
      ) : (
        <Button
          title={usesRubies ? `◆${price.toLocaleString()}` : `¥${price.toLocaleString()}`}
          variant={affordable ? 'primary' : 'default'}
          disabled={!affordable}
          busy={busy}
          subtitle={affordable ? undefined : 'Cannot afford'}
          onPress={onBuy}
        />
      )}
    </Card>
  );
}

function LiveryCard({
  livery,
  cash,
  rubies,
  busy,
  chassisId,
  onBuy,
  onEquip,
}: {
  livery: LiveryOption;
  cash: number;
  rubies: number;
  busy: boolean;
  chassisId: string;
  onBuy: () => void;
  onEquip: () => void;
}) {
  const { primary, secondary } = LiveryColors(livery);
  const usesRubies = livery.price_rubies > 0;
  const price = usesRubies ? livery.price_rubies : livery.price_cash;
  const affordable = usesRubies ? rubies >= price : cash >= price;

  return (
    <Card tone={livery.equipped ? 'accent' : 'default'}>
      <Row style={{ justifyContent: 'space-between' }}>
        <Text style={t.heading}>{livery.name}</Text>
        {livery.equipped && <Text style={[t.small, { color: colors.accent }]}>on the car</Text>}
      </Row>
      <View style={{ alignItems: 'center' }}>
        <CarArt primary={primary} secondary={secondary} width={84} chassisId={chassisId} />
      </View>
      {livery.owned ? (
        livery.equipped ? null : (
          <Button title="Apply" busy={busy} onPress={onEquip} />
        )
      ) : (
        <Button
          title={usesRubies ? `◆${price}` : `¥${price.toLocaleString()}`}
          variant={affordable ? 'primary' : 'default'}
          disabled={!affordable}
          busy={busy}
          onPress={onBuy}
        />
      )}
    </Card>
  );
}
