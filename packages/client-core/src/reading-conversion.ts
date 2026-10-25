/** Source-owned optional provenance seam pending official Release generation.
 * Parsed from actual response facts; never invents conversion support or grants.
 */
export type ReadingConversion = Readonly<{
  originalRevision:string; originalSourceId:string; originalFormat:'mobi'|'azw3'; originalSize:number;
  inputSHA256:string; converter:'libmobi:85dcfe803fc2a21020ddcf15c3eb66b93d388add'; workerSHA256:string;
  options:'portico-epub-v1;unencrypted-only;no-replica;default-kf8'; outputSHA256:string; outputSize:number; revision:string;
}>;
const keys=['originalRevision','originalSourceId','originalFormat','originalSize','inputSHA256','converter','workerSHA256','options','outputSHA256','outputSize','revision'] as const;
const record=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const hash=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const size=(value:unknown,max:number):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0&&value<=max;
/** The base still passes the canonical decoder, including its normal facts hook. */
export function withReadingConversion<T extends {format:string;sourceId:string;revision:string;size:number;sequential?:boolean}>(raw:unknown,decode:(value:unknown)=>T):T&Readonly<{conversion?:ReadingConversion}> {
  const plain=record(raw)?{...raw}:undefined;
  const supplied=plain?.conversion;
  if(plain)delete plain.conversion;
  const base=decode(plain??raw);
  if(supplied===undefined)return base;
  if(!record(supplied)||Object.keys(supplied).length!==keys.length||keys.some(key=>!Object.hasOwn(supplied,key))||Object.keys(supplied).some(key=>!keys.some(allowed=>allowed===key))
    ||base.format!=='epub'||base.sequential===true||supplied.originalSourceId!==base.sourceId
    ||!hash(supplied.originalRevision)||typeof supplied.originalSourceId!=='string'||!supplied.originalSourceId.length||supplied.originalSourceId.length>256
    ||(supplied.originalFormat!=='mobi'&&supplied.originalFormat!=='azw3')||!size(supplied.originalSize,64<<20)
    ||!hash(supplied.inputSHA256)||supplied.converter!=='libmobi:85dcfe803fc2a21020ddcf15c3eb66b93d388add'||!hash(supplied.workerSHA256)
    ||supplied.options!=='portico-epub-v1;unencrypted-only;no-replica;default-kf8'||!hash(supplied.outputSHA256)
    ||!size(supplied.outputSize,128<<20)||supplied.outputSize!==base.size||!hash(supplied.revision)||supplied.revision!==base.revision)throw new Error('invalid_reading_conversion');
  const conversion:ReadingConversion=Object.freeze({originalRevision:supplied.originalRevision,originalSourceId:supplied.originalSourceId,originalFormat:supplied.originalFormat,originalSize:supplied.originalSize,inputSHA256:supplied.inputSHA256,converter:supplied.converter,workerSHA256:supplied.workerSHA256,options:supplied.options,outputSHA256:supplied.outputSHA256,outputSize:supplied.outputSize,revision:supplied.revision});
  return Object.freeze({...base,conversion});
}
