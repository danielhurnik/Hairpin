import { Stack } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import React from 'react';
import { Text, View } from 'react-native';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { API_URL } from '../src/api';
import { Banner, Button, Loading, s } from '../src/components/ui';
import { GameProvider, useGame } from '../src/state';
import { colors, space, text as t } from '../src/theme';

export default function RootLayout() {
  return (
    <SafeAreaProvider>
      <StatusBar style="light" />
      <GameProvider>
        <Gate />
      </GameProvider>
    </SafeAreaProvider>
  );
}

/**
 * Nothing renders until the server has handed us a state bundle. The client
 * has no offline model of the game to fall back on — by design.
 */
function Gate() {
  const { loading, error, state, retry } = useGame();

  if (loading) {
    return (
      <View style={[s.screen, { justifyContent: 'center' }]}>
        <Loading label="Warming up…" />
      </View>
    );
  }

  if (error || !state) {
    return (
      <View style={[s.screen, { justifyContent: 'center', padding: space(6), gap: space(4) }]}>
        <Text style={t.title}>Can't reach the mountain</Text>
        <Banner tone="error">{error ?? 'No state returned.'}</Banner>
        <Text style={t.small}>
          The client is pointed at {API_URL}. Start the server with `go run ./cmd/hairpin` in
          server/, or set EXPO_PUBLIC_API_URL to its address.
        </Text>
        <Button title="Try again" variant="primary" onPress={retry} />
      </View>
    );
  }

  return (
    <Stack
      screenOptions={{
        headerShown: false,
        contentStyle: { backgroundColor: colors.bg },
      }}
    />
  );
}
