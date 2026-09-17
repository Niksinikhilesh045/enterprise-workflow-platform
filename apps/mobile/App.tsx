import React,{useState} from 'react';
import {SafeAreaView,StyleSheet,Text,TextInput,TouchableOpacity,View} from 'react-native';

const API='http://localhost:8080';
export default function App(){
 const [tenant,setTenant]=useState('acme'); const [recordId,setRecordId]=useState(''); const [version,setVersion]=useState('1'); const [status,setStatus]=useState('Ready');
 async function approve(){
  try{const r=await fetch(`${API}/api/v1/records/${recordId}`,{method:'PUT',headers:{'Content-Type':'application/json','X-Tenant-ID':tenant,'If-Match':version},body:JSON.stringify({state:'APPROVED'})}); const body=await r.json(); if(!r.ok){setStatus(body.error??'Update failed');return;} setVersion(String(body.version)); setStatus(`Approved ${body.id}`);}catch{setStatus('Cannot reach API. On a device, point API to your computer LAN address.');}
 }
 return <SafeAreaView style={s.page}><View style={s.hero}><Text style={s.kicker}>MOBILE APPROVALS</Text><Text style={s.title}>Workflow Inbox</Text><Text style={s.copy}>Approve tenant-scoped requests with optimistic concurrency protection.</Text></View><View style={s.card}><Text style={s.label}>Tenant</Text><TextInput value={tenant} onChangeText={setTenant} style={s.input}/><Text style={s.label}>Record ID</Text><TextInput value={recordId} onChangeText={setRecordId} placeholder="rec_000002" style={s.input}/><Text style={s.label}>Expected version</Text><TextInput value={version} onChangeText={setVersion} keyboardType="number-pad" style={s.input}/><TouchableOpacity style={s.button} onPress={approve}><Text style={s.buttonText}>Approve request</Text></TouchableOpacity><Text style={s.status}>{status}</Text></View></SafeAreaView>
}
const s=StyleSheet.create({page:{flex:1,backgroundColor:'#F3F6FB',padding:24},hero:{marginTop:48,marginBottom:28},kicker:{fontSize:12,fontWeight:'800',letterSpacing:2,color:'#596579'},title:{fontSize:36,fontWeight:'800',color:'#172033',marginTop:5},copy:{fontSize:16,color:'#697386',lineHeight:24,marginTop:8},card:{backgroundColor:'white',padding:22,borderRadius:20},label:{fontSize:13,fontWeight:'700',color:'#4A5568',marginTop:10},input:{borderWidth:1,borderColor:'#D7DFEA',borderRadius:10,padding:12,marginTop:6},button:{backgroundColor:'#172033',padding:15,borderRadius:11,marginTop:20,alignItems:'center'},buttonText:{color:'white',fontWeight:'800'},status:{color:'#596579',marginTop:18,lineHeight:20}});
