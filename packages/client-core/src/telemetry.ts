/**
 * Owner telemetry and transcode capacity, as the selected server publishes
 * them. Every metric carries its own status, so a client renders "not
 * available on this platform" instead of drawing a zero the server never
 * measured. Nothing here is inferred on the client.
 */
export type MetricStatus='available'|'limited'|'unavailable';
export type Metric={status:MetricStatus;value:number;detail?:string};
export type MetricName='cpu'|'memory'|'diskRead'|'diskWrite'|'netIn'|'netOut'|'gpuUsage'|'gpuMemory'|'gpuEncoder';
export const METRIC_NAMES:MetricName[]=['cpu','memory','diskRead','diskWrite','netIn','netOut','gpuUsage','gpuMemory','gpuEncoder'];
export type TelemetryWindow='10m'|'1h'|'24h';
export const TELEMETRY_WINDOWS:TelemetryWindow[]=['10m','1h','24h'];
export type TelemetrySample={at:number;metrics:Record<MetricName,Metric>;memoryUsedBytes:number;memoryTotalBytes:number;gpuDevice?:string;gpuProvider?:string};
export type TelemetryPoint={t:number;v:number};
export type TelemetryReading={window:TelemetryWindow;series:Record<MetricName,TelemetryPoint[]>;status:Record<MetricName,Metric>;observedAt:number};

export type HardwareProbe={backend:string;supported:boolean;detail:string};
export type HardwareStatus={configured:string;effective:string;device:string;probes:HardwareProbe[]};
export type SessionLimits={concurrent:number;hardware:number;software:number;background:number};
/** A null count means the server publishes no figure, never "no sessions". */
export type SessionCounts={active:number|null;hardware:number|null;software:number|null;background:number|null;limits:SessionLimits};
export type ToneMappingStatus={status:'disabled'|'available'|'limited'|'unavailable';detail:string};
export type TemporaryDirectory={path:string;ready:boolean;freeBytes:number|null};
export type TranscodePreset={id:string;label:string;height:number;videoKbps:number;audioKbps:number};
export type RuntimeDependency={path:string;version:string;ok:boolean};
export type TranscodeCapacity={
 enabled:boolean;hardware:HardwareStatus;sessions:SessionCounts;hdrToneMapping:ToneMappingStatus;
 directStreamRemux:boolean;x264Preset:string;planningPolicy:string;throttleBufferSeconds:number;playedRetentionSeconds:number;
 temporaryDirectory:TemporaryDirectory;presets:TranscodePreset[];dependencies:Record<string,RuntimeDependency>;warnings:string[];observedAt:number};

const obj=(v:unknown):v is Record<string,any>=>typeof v==='object'&&v!==null&&!Array.isArray(v);
const integer=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0;
const finite=(v:unknown):v is number=>typeof v==='number'&&Number.isFinite(v);
const text=(v:unknown,max=512):v is string=>typeof v==='string'&&v.length<=max;
function invalid():never{throw new Error('Invalid selected-server telemetry response.');}
function base(v:unknown):Record<string,any>{if(!obj(v))invalid();return v;}

export function parseMetric(value:unknown):Metric{
 const m=base(value);
 if(!['available','limited','unavailable'].includes(m.status)||!finite(m.value)||m.detail!==undefined&&!text(m.detail,512))invalid();
 if(m.status==='unavailable'&&m.value!==0)invalid();
 return m as Metric;
}
function metricMap(value:unknown):Record<MetricName,Metric>{
 const source=base(value);const out={} as Record<MetricName,Metric>;
 for(const name of METRIC_NAMES)out[name]=parseMetric(source[name]);
 return out;
}
export function parseTelemetrySample(value:unknown):TelemetrySample{
 const s=base(value);
 if(!integer(s.at)||!integer(s.memoryUsedBytes)||!integer(s.memoryTotalBytes)||s.memoryUsedBytes>s.memoryTotalBytes&&s.memoryTotalBytes!==0)invalid();
 for(const key of ['gpuDevice','gpuProvider'])if(s[key]!==undefined&&!text(s[key],200))invalid();
 return {...s,metrics:metricMap(s.metrics)} as TelemetrySample;
}
/** A window holds at most one point every two seconds for ten minutes. */
const MAX_POINTS=1600;
export function parseTelemetryReading(value:unknown):TelemetryReading{
 const r=base(value);
 if(!TELEMETRY_WINDOWS.includes(r.window)||!integer(r.observedAt))invalid();
 const series=base(r.series);const out={} as Record<MetricName,TelemetryPoint[]>;
 for(const name of METRIC_NAMES){
  const points=series[name];
  if(!Array.isArray(points)||points.length>MAX_POINTS)invalid();
  let previous=-1;
  out[name]=points.map(point=>{
   const p=base(point);
   if(!integer(p.t)||!finite(p.v)||p.t<previous||p.t>r.observedAt)invalid();
   previous=p.t;
   return {t:p.t,v:p.v};
  });
 }
 return {window:r.window,series:out,status:metricMap(r.status),observedAt:r.observedAt};
}

export function parseTranscodeCapacity(value:unknown):TranscodeCapacity{
 const c=base(value);
 if(typeof c.enabled!=='boolean'||typeof c.directStreamRemux!=='boolean'||!text(c.x264Preset,40)||!text(c.planningPolicy,40)||
  !integer(c.throttleBufferSeconds)||!integer(c.playedRetentionSeconds)||!integer(c.observedAt))invalid();
 const hardware=base(c.hardware);
 if(!text(hardware.configured,40)||!text(hardware.effective,40)||!text(hardware.device,200)||!Array.isArray(hardware.probes)||hardware.probes.length>20)invalid();
 for(const probe of hardware.probes){const p=base(probe);if(!text(p.backend,40)||typeof p.supported!=='boolean'||!text(p.detail,512))invalid();}
 const sessions=base(c.sessions);
 for(const key of ['active','hardware','software','background'])if(sessions[key]!==null&&!integer(sessions[key]))invalid();
 const limits=base(sessions.limits);
 for(const key of ['concurrent','hardware','software','background'])if(!integer(limits[key]))invalid();
 const tone=base(c.hdrToneMapping);
 if(!['disabled','available','limited','unavailable'].includes(tone.status)||!text(tone.detail,512))invalid();
 const directory=base(c.temporaryDirectory);
 if(!text(directory.path,1024)||typeof directory.ready!=='boolean'||directory.freeBytes!==null&&!integer(directory.freeBytes))invalid();
 if(!Array.isArray(c.presets)||c.presets.length>20)invalid();
 for(const preset of c.presets){const p=base(preset);if(!text(p.id,40)||!text(p.label,80)||!integer(p.height)||!integer(p.videoKbps)||!integer(p.audioKbps))invalid();}
 const dependencies=base(c.dependencies);
 if(Object.keys(dependencies).length>8)invalid();
 for(const dependency of Object.values(dependencies)){const d=base(dependency);if(!text(d.path,1024)||!text(d.version,512)||typeof d.ok!=='boolean')invalid();}
 if(!Array.isArray(c.warnings)||c.warnings.length>20||c.warnings.some((w:unknown)=>!text(w,512)))invalid();
 return c as TranscodeCapacity;
}
