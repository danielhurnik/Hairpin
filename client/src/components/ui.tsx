import React from 'react';
import {
  ActivityIndicator,
  Pressable,
  RefreshControl,
  ScrollView,
  StyleSheet,
  Text,
  View,
  type StyleProp,
  type ViewStyle,
} from 'react-native';
import { colors, radius, space, text as t } from '../theme';

export function Screen({
  children,
  onRefresh,
}: {
  children: React.ReactNode;
  onRefresh?: () => Promise<void>;
}) {
  const [refreshing, setRefreshing] = React.useState(false);
  const pull = React.useCallback(async () => {
    if (!onRefresh) return;
    setRefreshing(true);
    try {
      await onRefresh();
    } finally {
      setRefreshing(false);
    }
  }, [onRefresh]);

  return (
    <ScrollView
      style={s.screen}
      contentContainerStyle={s.screenContent}
      keyboardShouldPersistTaps="handled"
      indicatorStyle="white"
      refreshControl={
        onRefresh ? (
          <RefreshControl refreshing={refreshing} onRefresh={pull} tintColor={colors.accent} />
        ) : undefined
      }>
      {children}
      <View style={{ height: space(12) }} />
    </ScrollView>
  );
}

export function Card({
  children,
  style,
  tone,
}: {
  children: React.ReactNode;
  style?: StyleProp<ViewStyle>;
  tone?: 'default' | 'raised' | 'accent';
}) {
  return (
    <View
      style={[
        s.card,
        tone === 'raised' && { backgroundColor: colors.surfaceAlt },
        tone === 'accent' && { borderColor: colors.accentDim },
        style,
      ]}>
      {children}
    </View>
  );
}

export function SectionLabel({ children, right }: { children: React.ReactNode; right?: React.ReactNode }) {
  return (
    <View style={s.sectionLabel}>
      <Text style={t.label}>{children}</Text>
      {right}
    </View>
  );
}

export function Button({
  title,
  onPress,
  variant = 'default',
  disabled,
  busy,
  subtitle,
  style,
}: {
  title: string;
  onPress?: () => void;
  variant?: 'default' | 'primary' | 'danger' | 'ghost';
  disabled?: boolean;
  busy?: boolean;
  subtitle?: string;
  style?: StyleProp<ViewStyle>;
}) {
  const off = disabled || busy;
  return (
    <Pressable
      onPress={off ? undefined : onPress}
      disabled={off}
      style={({ pressed }) => [
        s.button,
        variant === 'primary' && s.buttonPrimary,
        variant === 'danger' && s.buttonDanger,
        variant === 'ghost' && s.buttonGhost,
        off && s.buttonOff,
        pressed && !off && s.buttonPressed,
        style,
      ]}>
      {busy ? (
        <ActivityIndicator color={variant === 'primary' ? colors.bg : colors.text} />
      ) : (
        <>
          <Text
            style={[
              s.buttonText,
              variant === 'primary' && { color: colors.bg },
              off && { color: colors.textFaint },
            ]}>
            {title}
          </Text>
          {subtitle ? (
            <Text
              style={[
                s.buttonSub,
                variant === 'primary' && { color: 'rgba(11,12,15,0.7)' },
                off && { color: colors.textFaint },
              ]}>
              {subtitle}
            </Text>
          ) : null}
        </>
      )}
    </Pressable>
  );
}

/** A labelled progress bar used for driver stats and XP. */
export function Bar({
  value,
  max,
  color = colors.accent,
  height = 6,
}: {
  value: number;
  max: number;
  color?: string;
  height?: number;
}) {
  const pct = max <= 0 ? 0 : Math.max(0, Math.min(1, value / max));
  return (
    <View style={[s.barTrack, { height, borderRadius: height / 2 }]}>
      <View
        style={{
          width: `${pct * 100}%`,
          height: '100%',
          backgroundColor: color,
          borderRadius: height / 2,
        }}
      />
    </View>
  );
}

export function Pill({
  children,
  color = colors.textDim,
  bg,
}: {
  children: React.ReactNode;
  color?: string;
  bg?: string;
}) {
  return (
    <View style={[s.pill, { borderColor: color, backgroundColor: bg ?? 'transparent' }]}>
      <Text style={[s.pillText, { color }]}>{children}</Text>
    </View>
  );
}

export function Row({
  children,
  style,
  gap = 2,
}: {
  children: React.ReactNode;
  style?: StyleProp<ViewStyle>;
  gap?: number;
}) {
  return <View style={[s.row, { gap: space(gap) }, style]}>{children}</View>;
}

export function Empty({ children }: { children: React.ReactNode }) {
  return (
    <View style={s.empty}>
      <Text style={[t.small, { textAlign: 'center' }]}>{children}</Text>
    </View>
  );
}

export function Banner({ tone, children }: { tone: 'error' | 'good' | 'info'; children: React.ReactNode }) {
  const color = tone === 'error' ? colors.red : tone === 'good' ? colors.green : colors.blue;
  return (
    <View style={[s.banner, { borderColor: color, backgroundColor: color + '18' }]}>
      <Text style={[t.small, { color }]}>{children}</Text>
    </View>
  );
}

export function Loading({ label }: { label?: string }) {
  return (
    <View style={s.loading}>
      <ActivityIndicator color={colors.accent} />
      {label ? <Text style={[t.small, { marginTop: space(3) }]}>{label}</Text> : null}
    </View>
  );
}

export const s = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  screenContent: { padding: space(4), gap: space(3) },
  card: {
    backgroundColor: colors.surface,
    borderRadius: radius.lg,
    borderWidth: 1,
    borderColor: colors.border,
    padding: space(4),
    gap: space(3),
  },
  sectionLabel: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginTop: space(2),
    marginBottom: space(1),
  },
  button: {
    backgroundColor: colors.raised,
    borderRadius: radius.md,
    borderWidth: 1,
    borderColor: colors.borderBright,
    paddingVertical: space(3),
    paddingHorizontal: space(4),
    alignItems: 'center',
    justifyContent: 'center',
    minHeight: 44,
  },
  buttonPrimary: { backgroundColor: colors.accent, borderColor: colors.accent },
  buttonDanger: { backgroundColor: 'transparent', borderColor: colors.red },
  buttonGhost: { backgroundColor: 'transparent', borderColor: colors.border },
  buttonOff: { backgroundColor: colors.surfaceAlt, borderColor: colors.border },
  buttonPressed: { opacity: 0.75 },
  buttonText: { color: colors.text, fontWeight: '700', fontSize: 14, letterSpacing: 0.4 },
  buttonSub: { color: colors.textDim, fontSize: 11, marginTop: 2 },
  barTrack: { backgroundColor: colors.raised, overflow: 'hidden', width: '100%' },
  pill: {
    borderWidth: 1,
    borderRadius: radius.pill,
    paddingHorizontal: space(2),
    paddingVertical: 2,
    alignSelf: 'flex-start',
  },
  pillText: { fontSize: 10, fontWeight: '700', letterSpacing: 0.6 },
  row: { flexDirection: 'row', alignItems: 'center' },
  empty: { padding: space(6), alignItems: 'center' },
  banner: { borderWidth: 1, borderRadius: radius.md, padding: space(3) },
  loading: { padding: space(10), alignItems: 'center', justifyContent: 'center' },
});
