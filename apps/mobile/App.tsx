import React, { useMemo, useState } from 'react';
import { SafeAreaView, StyleSheet, Text, TextInput, TouchableOpacity, View } from 'react-native';

const API = 'http://localhost:8080';

type TokenClaims = { sub?: string; tenantId?: string; role?: string; exp?: number };

function decodeBase64Url(value: string) {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/');
  let bits = '';
  for (const char of normalized) {
    const index = alphabet.indexOf(char);
    if (index >= 0) bits += index.toString(2).padStart(6, '0');
  }
  let output = '';
  for (let i = 0; i + 8 <= bits.length; i += 8) output += String.fromCharCode(parseInt(bits.slice(i, i + 8), 2));
  return output;
}

function decodeClaims(token: string): TokenClaims | null {
  try {
    const [payload] = token.split('.');
    if (!payload) return null;
    return JSON.parse(decodeBase64Url(payload)) as TokenClaims;
  } catch {
    return null;
  }
}

export default function App() {
  const [token, setToken] = useState('');
  const [recordId, setRecordId] = useState('');
  const [version, setVersion] = useState('1');
  const [status, setStatus] = useState('Paste an approver token to begin.');
  const claims = useMemo(() => decodeClaims(token.trim()), [token]);

  async function approve() {
    if (!token.trim()) {
      setStatus('Bearer token is required.');
      return;
    }
    if (!recordId.trim()) {
      setStatus('Record ID is required.');
      return;
    }
    try {
      const response = await fetch(`${API}/api/v1/records/${recordId.trim()}`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token.trim()}`,
          'If-Match': version,
        },
        body: JSON.stringify({ state: 'APPROVED' }),
      });
      const body = await response.json();
      if (!response.ok) {
        if (response.status === 401) setStatus('Authentication failed or token expired.');
        else if (response.status === 403) setStatus('This token is not authorized to approve requests.');
        else if (response.status === 409) setStatus('Version conflict. Refresh the record version and retry.');
        else setStatus(body.error ?? 'Update failed.');
        return;
      }
      setVersion(String(body.version));
      setStatus(`Approved ${body.id} as ${claims?.role ?? 'authorized user'}.`);
    } catch {
      setStatus('Cannot reach API. On a device, point API to your computer LAN address.');
    }
  }

  return (
    <SafeAreaView style={s.page}>
      <View style={s.hero}>
        <Text style={s.kicker}>MOBILE APPROVALS</Text>
        <Text style={s.title}>Workflow Inbox</Text>
        <Text style={s.copy}>Approve tenant-scoped requests with signed identity and optimistic concurrency protection.</Text>
      </View>
      <View style={s.card}>
        <Text style={s.label}>Approver bearer token</Text>
        <TextInput
          value={token}
          onChangeText={setToken}
          multiline
          autoCapitalize="none"
          autoCorrect={false}
          placeholder="Paste bearer token"
          style={[s.input, s.tokenInput]}
        />
        <View style={s.claims}>
          <Text style={s.claimText}>Tenant: {claims?.tenantId ?? '—'}</Text>
          <Text style={s.claimText}>Role: {claims?.role ?? '—'}</Text>
        </View>
        <Text style={s.label}>Record ID</Text>
        <TextInput value={recordId} onChangeText={setRecordId} placeholder="record id" autoCapitalize="none" style={s.input} />
        <Text style={s.label}>Expected version</Text>
        <TextInput value={version} onChangeText={setVersion} keyboardType="number-pad" style={s.input} />
        <TouchableOpacity style={s.button} onPress={approve}>
          <Text style={s.buttonText}>Approve request</Text>
        </TouchableOpacity>
        <Text style={s.status}>{status}</Text>
      </View>
    </SafeAreaView>
  );
}

const s = StyleSheet.create({
  page: { flex: 1, backgroundColor: '#F3F6FB', padding: 24 },
  hero: { marginTop: 48, marginBottom: 28 },
  kicker: { fontSize: 12, fontWeight: '800', letterSpacing: 2, color: '#596579' },
  title: { fontSize: 36, fontWeight: '800', color: '#172033', marginTop: 5 },
  copy: { fontSize: 16, color: '#697386', lineHeight: 24, marginTop: 8 },
  card: { backgroundColor: 'white', padding: 22, borderRadius: 20 },
  label: { fontSize: 13, fontWeight: '700', color: '#4A5568', marginTop: 10 },
  input: { borderWidth: 1, borderColor: '#D7DFEA', borderRadius: 10, padding: 12, marginTop: 6 },
  tokenInput: { minHeight: 82, textAlignVertical: 'top', fontSize: 12 },
  claims: { flexDirection: 'row', gap: 18, marginTop: 10 },
  claimText: { color: '#596579', fontSize: 13 },
  button: { backgroundColor: '#172033', padding: 15, borderRadius: 11, marginTop: 20, alignItems: 'center' },
  buttonText: { color: 'white', fontWeight: '800' },
  status: { color: '#596579', marginTop: 18, lineHeight: 20 },
});
