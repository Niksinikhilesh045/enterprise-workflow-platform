import React, { useMemo, useState } from 'react';
import {
  ActivityIndicator,
  FlatList,
  SafeAreaView,
  StyleSheet,
  Text,
  TextInput,
  TouchableOpacity,
  View,
} from 'react-native';

const API = 'http://localhost:8080';

type TokenClaims = { sub?: string; tenantId?: string; role?: string; exp?: number };
type WorkflowRecord = {
  id: string;
  workflowId: string;
  state: string;
  version: number;
  data?: Record<string, unknown>;
  updatedAt: string;
};

function decodeBase64Url(value: string) {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/');
  let bits = '';
  for (const char of normalized) {
    const index = alphabet.indexOf(char);
    if (index >= 0) bits += index.toString(2).padStart(6, '0');
  }
  let output = '';
  for (let i = 0; i + 8 <= bits.length; i += 8) {
    output += String.fromCharCode(parseInt(bits.slice(i, i + 8), 2));
  }
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

function requestSummary(record: WorkflowRecord) {
  const value = record.data?.request;
  if (typeof value === 'string' && value.trim()) return value;
  return `Request ${record.id}`;
}

export default function App() {
  const [token, setToken] = useState('');
  const [records, setRecords] = useState<WorkflowRecord[]>([]);
  const [status, setStatus] = useState('Paste an approver token, then refresh the inbox.');
  const [loading, setLoading] = useState(false);
  const [approvingID, setApprovingID] = useState<string | null>(null);
  const claims = useMemo(() => decodeClaims(token.trim()), [token]);

  function authHeaders(extra: Record<string, string> = {}) {
    return {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token.trim()}`,
      ...extra,
    };
  }

  async function refreshInbox() {
    if (!token.trim()) {
      setStatus('Bearer token is required.');
      setRecords([]);
      return;
    }
    setLoading(true);
    try {
      const response = await fetch(`${API}/api/v1/records?state=SUBMITTED`, {
        headers: authHeaders(),
      });
      const body = await response.json();
      if (!response.ok) {
        setRecords([]);
        if (response.status === 401) setStatus('Authentication failed or token expired.');
        else if (response.status === 403) setStatus('This token cannot read the approval inbox.');
        else setStatus(body.error ?? 'Unable to load requests.');
        return;
      }
      const items = body as WorkflowRecord[];
      setRecords(items);
      setStatus(items.length ? `${items.length} request${items.length === 1 ? '' : 's'} awaiting approval.` : 'No submitted requests are waiting.');
    } catch {
      setStatus('Cannot reach API. On a physical device, point API to your computer LAN address.');
    } finally {
      setLoading(false);
    }
  }

  async function approve(record: WorkflowRecord) {
    setApprovingID(record.id);
    try {
      const response = await fetch(`${API}/api/v1/records/${record.id}`, {
        method: 'PUT',
        headers: authHeaders({ 'If-Match': String(record.version) }),
        body: JSON.stringify({ state: 'APPROVED' }),
      });
      const body = await response.json();
      if (!response.ok) {
        if (response.status === 401) setStatus('Authentication failed or token expired.');
        else if (response.status === 403) setStatus('This token is not authorized to approve requests.');
        else if (response.status === 409) setStatus('That request changed before approval. Refresh the inbox and retry.');
        else setStatus(body.error ?? 'Approval failed.');
        return;
      }
      setRecords((current) => current.filter((item) => item.id !== record.id));
      setStatus(`Approved ${requestSummary(record)}.`);
    } catch {
      setStatus('Cannot reach API. On a physical device, point API to your computer LAN address.');
    } finally {
      setApprovingID(null);
    }
  }

  return (
    <SafeAreaView style={s.page}>
      <FlatList
        data={records}
        keyExtractor={(item) => item.id}
        contentContainerStyle={s.content}
        ListHeaderComponent={
          <>
            <View style={s.hero}>
              <Text style={s.kicker}>MOBILE APPROVALS</Text>
              <Text style={s.title}>Workflow Inbox</Text>
              <Text style={s.copy}>Review tenant-scoped requests with signed identity and optimistic concurrency protection.</Text>
            </View>
            <View style={s.authCard}>
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
              <TouchableOpacity style={s.refreshButton} onPress={refreshInbox} disabled={loading}>
                {loading ? <ActivityIndicator color="white" /> : <Text style={s.buttonText}>Refresh inbox</Text>}
              </TouchableOpacity>
              <Text style={s.status}>{status}</Text>
            </View>
            <View style={s.sectionHeader}>
              <Text style={s.sectionTitle}>Submitted requests</Text>
              <Text style={s.count}>{records.length}</Text>
            </View>
          </>
        }
        renderItem={({ item }) => (
          <View style={s.requestCard}>
            <View style={s.requestTopline}>
              <Text style={s.state}>{item.state}</Text>
              <Text style={s.version}>v{item.version}</Text>
            </View>
            <Text style={s.requestTitle}>{requestSummary(item)}</Text>
            <Text style={s.recordID}>{item.id}</Text>
            <Text style={s.timestamp}>Updated {new Date(item.updatedAt).toLocaleString()}</Text>
            <TouchableOpacity
              style={[s.button, approvingID === item.id && s.buttonDisabled]}
              onPress={() => approve(item)}
              disabled={approvingID !== null}
            >
              {approvingID === item.id ? <ActivityIndicator color="white" /> : <Text style={s.buttonText}>Approve</Text>}
            </TouchableOpacity>
          </View>
        )}
        ListEmptyComponent={!loading ? <Text style={s.empty}>Refresh to load submitted requests for this tenant.</Text> : <View />}
      />
    </SafeAreaView>
  );
}

const s = StyleSheet.create({
  page: { flex: 1, backgroundColor: '#F3F6FB' },
  content: { padding: 24, paddingBottom: 48 },
  hero: { marginTop: 28, marginBottom: 22 },
  kicker: { fontSize: 12, fontWeight: '800', letterSpacing: 2, color: '#596579' },
  title: { fontSize: 36, fontWeight: '800', color: '#172033', marginTop: 5 },
  copy: { fontSize: 16, color: '#697386', lineHeight: 24, marginTop: 8 },
  authCard: { backgroundColor: 'white', padding: 20, borderRadius: 20 },
  label: { fontSize: 13, fontWeight: '700', color: '#4A5568' },
  input: { borderWidth: 1, borderColor: '#D7DFEA', borderRadius: 10, padding: 12, marginTop: 6 },
  tokenInput: { minHeight: 76, textAlignVertical: 'top', fontSize: 12 },
  claims: { flexDirection: 'row', gap: 18, marginTop: 10 },
  claimText: { color: '#596579', fontSize: 13 },
  refreshButton: { backgroundColor: '#334155', padding: 14, borderRadius: 11, marginTop: 18, alignItems: 'center' },
  status: { color: '#596579', marginTop: 14, lineHeight: 20 },
  sectionHeader: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', marginTop: 28, marginBottom: 12 },
  sectionTitle: { fontSize: 20, fontWeight: '800', color: '#172033' },
  count: { backgroundColor: '#E6ECF5', color: '#334155', paddingHorizontal: 10, paddingVertical: 4, borderRadius: 20, fontWeight: '800' },
  requestCard: { backgroundColor: 'white', padding: 20, borderRadius: 18, marginBottom: 14 },
  requestTopline: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
  state: { fontSize: 11, fontWeight: '800', letterSpacing: 1, color: '#475569' },
  version: { color: '#64748B', fontSize: 12 },
  requestTitle: { fontSize: 21, fontWeight: '800', color: '#172033', marginTop: 12 },
  recordID: { color: '#64748B', fontSize: 12, marginTop: 6 },
  timestamp: { color: '#94A3B8', fontSize: 12, marginTop: 5 },
  button: { backgroundColor: '#172033', padding: 14, borderRadius: 11, marginTop: 18, alignItems: 'center' },
  buttonDisabled: { opacity: 0.6 },
  buttonText: { color: 'white', fontWeight: '800' },
  empty: { color: '#64748B', textAlign: 'center', paddingVertical: 28 },
});
